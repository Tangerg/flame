package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/web/internal/contract"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/flame/plugins/web/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/httpreq"
	"github.com/Tangerg/scope/tools/web"
)

func readTool[P, R any](name, description string, read func(context.Context, P) (R, error)) (tool.Tool, error) {
	return tool.NewFunc(tool.FuncConfig{Name: name, Description: description}, func(ctx context.Context, input P) (R, error) {
		result, err := read(ctx, input)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return result, errors.Join(err, ctx.Err(), context.Cause(ctx))
		}
		kind, message := tool.FailureKindFailed, "web: read failed"
		if refusal, known := errors.AsType[*portable.Refusal](err); known {
			kind, message = tool.FailureKindRejected, refusal.Message
		} else if classify(err) != "" {
			kind, message = tool.FailureKindRejected, err.Error()
		}
		// Cache publication cannot create an unknown remote mutation outcome.
		failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: kind, Output: chat.NewTextToolOutput(message), Cause: err})
		return result, errors.Join(failure, failureErr)
	})
}

func newServer(research *research, requests *requests) (*sdk.Server, error) {
	if research == nil || requests == nil {
		return nil, errors.New("web: research and request owners are required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.web", Version: "2.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	search, err := readTool("web_search", contracts.Search.Descriptor().Description, research.searchWeb)
	if err != nil {
		return nil, err
	}
	fetch, err := readTool("web_fetch", contracts.Fetch.Descriptor().Description, research.fetchPage)
	if err != nil {
		return nil, err
	}
	browse, err := readTool("web_browse", contracts.Browse.Descriptor().Description, research.browse)
	if err != nil {
		return nil, err
	}
	request, err := tool.NewFunc(tool.FuncConfig{Name: "http_request", Description: contracts.Request.Descriptor().Description}, func(ctx context.Context, _ httpreq.Request) (wire.HTTPResponse, error) {
		id, err := pluginsdk.ToolCommandID(ctx)
		if err != nil {
			return wire.HTTPResponse{}, err
		}
		input, err := qimumcp.RequestInput(ctx)
		if err != nil {
			return wire.HTTPResponse{}, err
		}
		response, err := requests.execute(ctx, contracts.Request.Name(), id, input)
		return response, requestToolFailure(err)
	})
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, search, fetch, browse, request); err != nil {
		return nil, err
	}
	toolReceipts, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, requests.db),
		contracts.Request.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[httpreq.Request]) (wire.HTTPResponse, error) {
			input, err := command.RequestInput()
			if err != nil {
				return wire.HTTPResponse{}, err
			}
			return requests.execute(ctx, contracts.Request.Name(), command.ID, input)
		}),
		toolReceipts.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			input := command.Input
			if input.Method != "http_request" {
				return pluginsdk.ToolReceipt{}, nil
			}
			input.Method = contracts.Request.Name()
			return requests.receipt(ctx, input)
		}),
		contracts.Search.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[web.SearchRequest]) (web.SearchResponse, error) {
			return research.searchWeb(ctx, command.Input)
		}),
		contracts.Fetch.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[web.FetchRequest]) (capture.Page, error) {
			return research.fetchPage(ctx, command.Input)
		}),
		contracts.Browse.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[wire.BrowseInput]) (capture.Page, error) {
			return research.browse(ctx, command.Input)
		}),
		backend.BindCapability("web", contracts.Capability),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}
