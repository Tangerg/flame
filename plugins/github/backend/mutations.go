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

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"

	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/tool"
)

func decodeWriteReply[R any](method *frame.Method, reply []byte) (R, error) {
	var result R
	raw, err := pluginmcp.DecodeReply[jsontext.Value](reply)
	if err != nil {
		return result, err
	}
	if err := method.ValidateOutput(raw); err != nil {
		return result, err
	}
	err = json.Unmarshal(raw, &result, json.RejectUnknownMembers(true), framevalue.Numbers())
	return result, err
}

func mutate[P, R any](ctx context.Context, c *connector, method *frame.Method, id string, admitted framevalue.JSON, call func(context.Context, P) (R, error)) (R, error) {
	var zero R
	var input P
	prepared, original, err := pluginstorage.AdmitCommand(ctx, c.store.db, method.Name(), id, admitted, func(ctx context.Context) error {
		if err := method.ValidateInput(admitted); err != nil {
			return err
		}
		if err := admitted.Decode(&input); err != nil {
			return err
		}
		// Invalidation joins admission so a lost acknowledgement cannot preserve a
		// pre-write cache.
		_, err := c.store.db.Executor(ctx).ExecContext(ctx, `DELETE FROM responses`)
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
		encoded, err := framevalue.EncodeJSON(value)
		if err != nil {
			return zero, err
		}
		if err := method.ValidateOutput(encoded.Bytes()); err != nil {
			return zero, err
		}
	}
	reply, err := pluginmcp.EncodeReply(value, outcomeErr, classify)
	if err != nil {
		return zero, err
	}
	if err := prepared.SettleExternal(ctx, reply.Bytes(), nil); err != nil {
		return zero, err
	}
	return decodeWriteReply[R](method, reply.Bytes())
}

type toolReceiptReader func(context.Context, pluginmcp.ReceiptRequest) (pluginsdk.ToolReceipt, error)

func registerWrite[P, R any](backend *pluginmcp.Backend, tools *[]tool.Tool, receipts map[string]toolReceiptReader, c *connector, method *pluginmcp.Method[P, R], name string, call func(context.Context, P) (R, error)) error {
	contract, err := frame.AdmitMethod(method.Descriptor())
	if err != nil {
		return err
	}
	model, err := tool.NewFunc(tool.FuncConfig{Name: name, Description: method.Descriptor().Description}, func(ctx context.Context, _ P) (R, error) {
		var zero R
		id, err := pluginsdk.ToolCommandID(ctx)
		if err != nil {
			return zero, err
		}
		original, err := pluginmcp.RequestInput(ctx)
		if err != nil {
			return zero, err
		}
		value, err := mutate(ctx, c, contract, id, original, call)
		return value, writeFailure(err)
	})
	if err != nil {
		return err
	}
	if err := method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[P]) (R, error) {
		original, err := command.RequestInput()
		if err != nil {
			var zero R
			return zero, err
		}
		return mutate(ctx, c, contract, string(command.ID), original, call)
	}); err != nil {
		return err
	}
	receipts[name] = func(ctx context.Context, input pluginmcp.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
		input.Method = method.Name()
		receipt, err := pluginstorage.ReadReceipt(ctx, c.store.db, input)
		if err != nil || !receipt.Found {
			return pluginsdk.ToolReceipt{}, err
		}
		value, err := decodeWriteReply[R](contract, receipt.Reply)
		return pluginsdk.ToolReceiptFrom(value, err, writeFailure)
	}
	*tools = append(*tools, model)
	return nil
}

func writeFailure(err error) error {
	if _, refused := classify(err); err == nil || !refused {
		return err
	}
	return tool.FailureFromCause(tool.FailureKindRejected, err)
}

func (c *connector) sendWrite(ctx context.Context, method, path string, payload any, status int) (body []byte, err error) {
	if !c.credential.configured() {
		return nil, refused(refusalCredentialRequired, "a configured token is required for writes")
	}
	var encoded []byte
	if payload != nil {
		encoded, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		if len(encoded) > contractwire.MaxWriteBytes {
			return nil, invalidInput("write payload exceeds one MiB")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, apiOrigin+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	// No replayable body or synthetic idempotency header: GitHub does not own
	// our command key. An uncertain delivery can only read its private receipt.
	request.GetBody = nil
	c.identify(request)
	request.Header.Set("Content-Type", "application/json")
	upstream, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("github: write request: %w", err)
	}
	defer func() { err = errors.Join(err, upstream.Body.Close()) }()
	if upstream.StatusCode != status {
		return nil, writeRefusal(upstream)
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

func writeRefusal(upstream *http.Response) error {
	switch upstream.StatusCode {
	case http.StatusUnauthorized:
		return refused(refusalUnauthorized, "credential was rejected")
	case http.StatusForbidden, http.StatusTooManyRequests:
		return accessRefusal(upstream)
	case http.StatusNotFound:
		return refused(refusalNotFound, "resource is absent or not visible to this credential")
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return refused(refusalInvalidRequest, "GitHub rejected the write parameters")
	case http.StatusConflict:
		return refused(refusalConflict, "GitHub refused the current resource state or head SHA")
	case http.StatusMethodNotAllowed:
		return refused(refusalNotAllowed, "GitHub does not allow this operation on the current resource")
	default:
		return fmt.Errorf("github: unconfirmed write HTTP status %d", upstream.StatusCode)
	}
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
