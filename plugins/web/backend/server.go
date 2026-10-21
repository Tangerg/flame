package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	capture "github.com/Tangerg/flame/plugins/web/backend/domain"
	"github.com/Tangerg/flame/plugins/web/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/web"
)

func classify(err error) string {
	if errors.Is(err, errStoredSnapshot) {
		return ""
	}
	return rpc.Classify(err)
}

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
		if refusal, known := errors.AsType[*capability.Refusal](err); known {
			kind, message = tool.FailureKindRejected, refusal.Message
		} else if classify(err) != "" {
			kind, message = tool.FailureKindRejected, err.Error()
		}
		// Cache publication cannot create an unknown remote mutation outcome.
		failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: kind, Output: chat.NewTextToolOutput(message), Cause: err})
		return result, errors.Join(failure, failureErr)
	})
}

func newServer(research *research) (*sdk.Server, error) {
	if research == nil {
		return nil, errors.New("web: research owner is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.web", Version: "1.0.0"}, nil)
	backend, err := pluginsdk.NewBackend(server)
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
	if err := pluginsdk.RegisterTools(server, search, fetch, browse); err != nil {
		return nil, err
	}
	if err := errors.Join(
		contracts.Search.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[web.SearchRequest]) (web.SearchResponse, error) {
			return research.searchWeb(ctx, command.Input)
		}),
		contracts.Fetch.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[web.FetchRequest]) (capture.Page, error) {
			return research.fetchPage(ctx, command.Input)
		}),
		contracts.Browse.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.BrowseInput]) (capture.Page, error) {
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
