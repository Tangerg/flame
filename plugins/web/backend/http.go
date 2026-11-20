package main

import (
	"context"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/web/internal/wire"

	framecall "github.com/Tangerg/frame/call"
	"github.com/Tangerg/frame/sqlite"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/httpreq"
)

type requests struct {
	db      sqlite.Database
	binding *tool.Binding
}

const (
	refusalFetchNotConfigured  = "fetch_not_configured"
	refusalHTTPUnconfigured    = "http_unconfigured"
	refusalRequestRejected     = "request_rejected"
	refusalResponseTooLarge    = "response_too_large"
	refusalSearchNotConfigured = "search_not_configured"
)

func newRequests(db sqlite.Database, config httpreq.ClientConfig) (*requests, error) {
	if !db.Valid() {
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

func (r *requests) execute(ctx context.Context, method, id string, input framevalue.JSON) (wire.HTTPResponse, error) {
	var invocation tool.Invocation
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, r.db, method, id, input, func(context.Context) error {
		if r.binding == nil {
			return &framecall.Refusal{Code: refusalHTTPUnconfigured, Message: "web: configure http-allowed-hosts before making HTTP requests"}
		}
		var err error
		invocation, err = r.binding.Contract().Prepare((chat.ToolCall{ID: id, Name: requestTool, Arguments: input.String()}).Request(), tool.Admission{})
		if err != nil {
			return &framecall.Refusal{Code: pluginmcp.InvalidInputCode, Message: "web: invalid HTTP request"}
		}
		return nil
	})
	if err != nil {
		return wire.HTTPResponse{}, err
	}
	if prepared == nil {
		return pluginmcp.DecodeReply[wire.HTTPResponse](reply)
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
		callErr = &framecall.Refusal{Code: refusalRequestRejected, Message: message}
	}
	encoded, err := pluginmcp.EncodeReply(response, callErr, nil)
	if err != nil {
		return wire.HTTPResponse{}, err
	}
	if err := prepared.SettleExternal(ctx, encoded.Bytes(), nil); err != nil {
		return wire.HTTPResponse{}, err
	}
	return pluginmcp.DecodeReply[wire.HTTPResponse](encoded.Bytes())
}

func (r *requests) receipt(ctx context.Context, input pluginmcp.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
	receipt, err := pluginstorage.ReadReceipt(ctx, r.db, input)
	if err != nil || !receipt.Found {
		return pluginsdk.ToolReceipt{}, err
	}
	result, err := pluginmcp.DecodeReply[wire.HTTPResponse](receipt.Reply)
	return pluginsdk.ToolReceiptFrom(result, err, pluginsdk.RefusedToolFailure)
}
