package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func decodeWriteReply[R any](method *extension.Method, reply []byte) (R, error) {
	var result R
	raw, err := pluginsdk.DecodeReply[jsontext.Value](reply)
	if err != nil {
		return result, err
	}
	if err := method.ValidateOutput(raw); err != nil {
		return result, err
	}
	err = json.Unmarshal(raw, &result, json.RejectUnknownMembers(true))
	return result, err
}

func mutate[P, R any](ctx context.Context, c *connector, method *extension.Method, id string, input P, admitted jsonvalue.Value, call func(context.Context, P) (R, error)) (R, error) {
	var zero R
	prepared, original, err := pluginstorage.AdmitCommand(ctx, c.store.db, method.Name(), id, admitted, func(ctx context.Context) error {
		// Any dispatched write can affect several query pages. Invalidation joins
		// admission so a lost acknowledgement cannot preserve a pre-write cache.
		_, err := sqlite.Connection(ctx, c.store.db).ExecContext(ctx, `DELETE FROM responses`)
		return err
	})
	if err != nil {
		return zero, err
	}
	if prepared == nil {
		return decodeWriteReply[R](method, original)
	}
	if !prepared.NewlyAdmitted() {
		return zero, pluginstorage.ErrCommandUnconfirmed
	}
	value, outcomeErr := call(ctx, input)
	if outcomeErr == nil {
		encoded, err := jsonvalue.Encode(value)
		if err != nil {
			return zero, err
		}
		if err := method.ValidateOutput(encoded.JSON()); err != nil {
			return zero, err
		}
	}
	reply, err := pluginsdk.EncodeReply(value, outcomeErr, classify)
	if err != nil {
		return zero, err
	}
	if err := sqlite.RunInTx(ctx, c.store.db, func(ctx context.Context) error { return prepared.Complete(ctx, reply.JSON()) }); err != nil {
		return zero, err
	}
	return decodeWriteReply[R](method, reply.JSON())
}

type toolReceiptReader func(context.Context, pluginsdk.ReceiptRequest) (pluginsdk.ToolReceipt, error)

func registerWrite[P, R any](backend *pluginsdk.Backend, tools *[]tool.Tool, receipts map[string]toolReceiptReader, c *connector, method *pluginsdk.Method[P, R], name string, call func(context.Context, P) (R, error)) error {
	contract, err := extension.AdmitMethod(method.Descriptor())
	if err != nil {
		return err
	}
	model, err := tool.NewFunc(tool.FuncConfig{Name: name, Description: method.Descriptor().Description}, func(ctx context.Context, input P) (R, error) {
		var zero R
		id, err := pluginsdk.ToolCommandID(ctx)
		if err != nil {
			return zero, err
		}
		original, err := pluginsdk.RequestInput(ctx)
		if err != nil {
			return zero, err
		}
		value, err := mutate(ctx, c, contract, id, input, original, call)
		return value, writeFailure(err)
	})
	if err != nil {
		return err
	}
	if err := method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[P]) (R, error) {
		original, err := command.RequestInput()
		if err != nil {
			var zero R
			return zero, err
		}
		return mutate(ctx, c, contract, command.ID, command.Input, original, call)
	}); err != nil {
		return err
	}
	receipts[name] = func(ctx context.Context, input pluginsdk.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
		input.Method = method.Name()
		receipt, err := pluginstorage.ReadReceipt(ctx, c.store.db, input)
		if err != nil || !receipt.Found {
			return pluginsdk.ToolReceipt{}, err
		}
		value, err := decodeWriteReply[R](contract, receipt.Reply)
		if err != nil {
			failure, known := tool.ClassifyFailure(writeFailure(err))
			if !known {
				return pluginsdk.ToolReceipt{}, err
			}
			return pluginsdk.NewToolReceipt(failure.Output(), true)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		output, err := chat.NewJSONToolOutput(encoded)
		if err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		return pluginsdk.NewToolReceipt(output, false)
	}
	*tools = append(*tools, model)
	return nil
}

func writeFailure(err error) error {
	if err == nil || classify(err) == "" {
		return err
	}
	failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindRejected, Output: chat.NewTextToolOutput(err.Error()), Cause: err})
	if failureErr != nil {
		return errors.Join(failureErr, err)
	}
	return failure
}

func (c *connector) sendWrite(ctx context.Context, method, path string, payload any, status int) (body []byte, err error) {
	if !c.credential.configured() {
		return nil, refused("credential_required", "a configured token is required for writes")
	}
	var encoded []byte
	if payload != nil {
		encoded, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		if len(encoded) > rpc.MaxWriteBytes {
			return nil, refused("invalid_input", "write payload exceeds one MiB")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, apiOrigin+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	// No replayable body or synthetic idempotency header: GitHub does not own
	// our command key. An uncertain delivery can only read its private receipt.
	request.GetBody = nil
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Api-Version", apiVersion)
	request.Header.Set("User-Agent", "Flame-GitHub-Plugin/1.0")
	request.Header.Set("Authorization", c.credential.authorization())
	upstream, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("github: write request: %w", err)
	}
	defer func() { err = errors.Join(err, upstream.Body.Close()) }()
	if upstream.StatusCode != status {
		switch upstream.StatusCode {
		case http.StatusUnauthorized:
			return nil, refused("unauthorized", "credential was rejected")
		case http.StatusForbidden, http.StatusTooManyRequests:
			if upstream.StatusCode == http.StatusTooManyRequests || upstream.Header.Get("X-RateLimit-Remaining") == "0" || upstream.Header.Get("Retry-After") != "" {
				return nil, refused("rate_limited", "write was rate limited; no retry was attempted")
			}
			return nil, refused("forbidden", "write was refused by GitHub permissions or policy")
		case http.StatusNotFound:
			return nil, refused("not_found", "resource is absent or not visible to this credential")
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return nil, refused("invalid_request", "GitHub rejected the write parameters")
		case http.StatusConflict:
			return nil, refused("conflict", "GitHub refused the current resource state or head SHA")
		case http.StatusMethodNotAllowed:
			return nil, refused("not_allowed", "GitHub does not allow this operation on the current resource")
		default:
			return nil, fmt.Errorf("github: unconfirmed write HTTP status %d", upstream.StatusCode)
		}
	}
	body, err = io.ReadAll(io.LimitReader(upstream.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("github: write acknowledgement exceeds four MiB")
	}
	return body, nil
}

func writeValue[W, R any](ctx context.Context, c *connector, method, path string, payload any, status int, admit func(W) (R, error)) (R, error) {
	var zero R
	body, err := c.sendWrite(ctx, method, path, payload, status)
	if err != nil {
		return zero, err
	}
	if string(bytes.TrimSpace(body)) == "null" {
		return zero, errors.New("github: write acknowledgement must be an object")
	}
	var wire W
	if err := json.Unmarshal(body, &wire); err != nil {
		return zero, err
	}
	return admit(wire)
}
