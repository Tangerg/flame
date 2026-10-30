package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/web/internal/wire"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/httpreq"
)

type requests struct {
	db      *sql.DB
	binding *tool.Binding
}

func newRequests(db *sql.DB, config httpreq.ClientConfig) (*requests, error) {
	if db == nil {
		return nil, errors.New("web: request storage is required")
	}
	r := &requests{db: db}
	if len(config.AllowedHosts) == 0 {
		return r, nil
	}
	client, err := httpreq.NewClient(config)
	if err != nil {
		return nil, err
	}
	executable, err := httpreq.NewTool(client)
	if err != nil {
		return nil, err
	}
	binding, err := tool.Bind(executable)
	if err != nil {
		return nil, err
	}
	r.binding = &binding
	return r, nil
}

func (r *requests) execute(ctx context.Context, method, id string, input jsonvalue.Value) (wire.HTTPResponse, error) {
	var invocation tool.Invocation
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, r.db, method, id, input, func(context.Context) error {
		if r.binding == nil {
			return &portable.Refusal{Code: "http_unconfigured", Message: "web: configure http-allowed-hosts before making HTTP requests"}
		}
		var err error
		invocation, err = r.binding.Contract().Prepare(chat.ToolCall{ID: id, Name: "http_request", Arguments: input.String()}, nil)
		if err != nil {
			return &portable.Refusal{Code: "invalid_request", Message: "web: invalid HTTP request"}
		}
		return nil
	})
	if err != nil {
		return wire.HTTPResponse{}, err
	}
	if prepared == nil {
		return qimumcp.DecodeReply[wire.HTTPResponse](reply)
	}
	if !prepared.NewlyAdmitted() {
		return wire.HTTPResponse{}, pluginstorage.ErrCommandUnconfirmed
	}
	output, callErr := r.binding.Call(ctx, invocation)
	var response wire.HTTPResponse
	if callErr == nil {
		if err := json.Unmarshal(output.Output.Details, &response, json.RejectUnknownMembers(true)); err != nil {
			return wire.HTTPResponse{}, err
		}
	} else {
		// Only Scope's complete pre-dispatch classification proves a refusal.
		// Transport, redirect and body failures may follow an external effect.
		failure, known := tool.ClassifyFailure(callErr)
		if !known {
			return wire.HTTPResponse{}, callErr
		}
		message, text := failure.Output().Text()
		if !text {
			return wire.HTTPResponse{}, callErr
		}
		callErr = &portable.Refusal{Code: "request_rejected", Message: message}
	}
	encoded, err := qimumcp.EncodeReply(response, callErr, nil)
	if err != nil {
		return wire.HTTPResponse{}, err
	}
	if err := prepared.Complete(ctx, encoded.JSON()); err != nil {
		return wire.HTTPResponse{}, err
	}
	return qimumcp.DecodeReply[wire.HTTPResponse](encoded.JSON())
}

func requestToolFailure(err error) error {
	refusal, known := errors.AsType[*portable.Refusal](err)
	if !known {
		return err
	}
	failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindRejected, Output: chat.NewTextToolOutput(refusal.Message), Cause: err})
	return errors.Join(failure, failureErr)
}

func (r *requests) receipt(ctx context.Context, input qimumcp.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
	receipt, err := pluginstorage.ReadReceipt(ctx, r.db, input)
	if err != nil || !receipt.Found {
		return pluginsdk.ToolReceipt{}, err
	}
	result, err := qimumcp.DecodeReply[wire.HTTPResponse](receipt.Reply)
	if err != nil {
		failure, known := tool.ClassifyFailure(requestToolFailure(err))
		if !known {
			return pluginsdk.ToolReceipt{}, err
		}
		return pluginsdk.NewToolReceipt(failure.Output(), true)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	output, err := chat.NewJSONToolOutput(encoded)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	return pluginsdk.NewToolReceipt(output, false)
}
