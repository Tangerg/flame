package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/web/internal/capture"
	rpc "github.com/Tangerg/flame/plugins/web/internal/contract"
	"github.com/Tangerg/flame/plugins/web/internal/wire"
	framecall "github.com/Tangerg/frame/call"
	sdk "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/httpreq"
	"github.com/Tangerg/scope/tools/web"
)

func readTool[P, R any](name, description string, read func(context.Context, P) (R, error)) (tool.Tool, error) {
	return tool.NewFunc(tool.FuncConfig{Name: name, Description: description, Effect: tool.EffectSafe}, func(ctx context.Context, input P) (R, error) {
		result, err := read(ctx, input)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return result, errors.Join(err, ctx.Err(), context.Cause(ctx))
		}
		kind, message := tool.FailureKindFailed, "web: read failed"
		if refusal, known := errors.AsType[*framecall.Refusal](err); known {
			kind, message = tool.FailureKindRejected, refusal.Message
		} else if _, refused := classify(err); refused {
			kind, message = tool.FailureKindRejected, err.Error()
		}
		failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: kind, Output: tool.NewTextOutput(message), Cause: err})
		return result, errors.Join(failure, failureErr)
	})
}

const (
	searchTool       = "web_search"
	fetchTool        = "web_fetch"
	browseTool       = "web_browse"
	requestTool      = "http_request"
	capabilityExport = "web"
)

func newServer(research *research, requests *requests) (*sdk.Server, error) {
	if research == nil || requests == nil {
		return nil, errors.New("web: research and request owners are required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.web", Version: "2.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	tools, err := webTools(contracts, research, requests)
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, tools...); err != nil {
		return nil, err
	}
	if err := registerMethods(backend, contracts, research, requests); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

func webTools(contracts rpc.Contracts, research *research, requests *requests) ([]tool.Tool, error) {
	search, err := readTool(searchTool, contracts.Search.Descriptor().Description, research.searchWeb)
	if err != nil {
		return nil, err
	}
	fetch, err := readTool(fetchTool, contracts.Fetch.Descriptor().Description, research.fetchPage)
	if err != nil {
		return nil, err
	}
	browse, err := readTool(browseTool, contracts.Browse.Descriptor().Description, research.browse)
	if err != nil {
		return nil, err
	}
	request, err := tool.NewFunc(tool.FuncConfig{Name: requestTool, Description: contracts.Request.Descriptor().Description}, func(ctx context.Context, _ httpreq.Request) (wire.HTTPResponse, error) {
		id, err := pluginsdk.ToolCommandID(ctx)
		if err != nil {
			return wire.HTTPResponse{}, err
		}
		input, err := pluginmcp.RequestInput(ctx)
		if err != nil {
			return wire.HTTPResponse{}, err
		}
		response, err := requests.execute(ctx, contracts.Request.Name(), id, input)
		return response, pluginsdk.RefusedToolFailure(err)
	})
	if err != nil {
		return nil, err
	}
	return []tool.Tool{search, fetch, browse, request}, nil
}

func registerMethods(backend *pluginmcp.Backend, contracts rpc.Contracts, research *research, requests *requests) error {
	toolReceipts, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return err
	}
	return errors.Join(
		pluginstorage.RegisterReceipts(backend, requests.db),
		contracts.Request.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[httpreq.Request]) (wire.HTTPResponse, error) {
			input, err := command.RequestInput()
			if err != nil {
				return wire.HTTPResponse{}, err
			}
			return requests.execute(ctx, contracts.Request.Name(), string(command.ID), input)
		}),
		toolReceipts.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[pluginmcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			input := command.Input
			if input.Method != requestTool {
				return pluginsdk.ToolReceipt{}, nil
			}
			input.Method = contracts.Request.Name()
			return requests.receipt(ctx, input)
		}),
		contracts.Search.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[web.SearchRequest]) (wire.SearchResponse, error) {
			return research.searchWeb(ctx, command.Input)
		}),
		contracts.Fetch.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[web.FetchRequest]) (capture.Page, error) {
			return research.fetchPage(ctx, command.Input)
		}),
		contracts.Browse.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.BrowseInput]) (capture.Page, error) {
			return research.browse(ctx, command.Input)
		}),
		backend.BindCapability(capabilityExport, contracts.Capability),
	)
}
