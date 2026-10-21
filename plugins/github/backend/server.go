package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	resource "github.com/Tangerg/flame/plugins/github/backend/domain"
	"github.com/Tangerg/flame/plugins/github/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func classify(err error) string {
	if errors.Is(err, resource.ErrResource) {
		return "invalid_resource"
	}
	return ""
}

func toolQuery[P, R any](name, description string, call func(context.Context, P) (R, error)) (tool.Tool, error) {
	return tool.NewFunc(tool.FuncConfig{Name: name, Description: description}, func(ctx context.Context, input P) (R, error) {
		result, err := call(ctx, input)
		_, known := errors.AsType[*capability.Refusal](err)
		if err == nil || !known && classify(err) == "" {
			return result, err
		}
		failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindRejected, Output: chat.NewTextToolOutput(err.Error()), Cause: err})
		return result, errors.Join(failure, failureErr)
	})
}

func newServer(connector *connector) (*sdk.Server, error) {
	if connector == nil {
		return nil, errors.New("github: connector is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.github", Version: "1.0.0"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return nil, err
	}
	list, err := toolQuery("list_issues", contracts.List.Descriptor().Description, connector.listIssues)
	if err != nil {
		return nil, err
	}
	issue, err := toolQuery("get_issue", contracts.Issue.Descriptor().Description, connector.getIssue)
	if err != nil {
		return nil, err
	}
	file, err := toolQuery("read_file", contracts.File.Descriptor().Description, connector.readFile)
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, list, issue, file); err != nil {
		return nil, err
	}
	if err := errors.Join(
		contracts.List.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.ListInput]) (rpc.IssuePage, error) {
			return connector.listIssues(ctx, command.Input)
		}),
		contracts.Issue.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.IssueInput]) (rpc.Issue, error) {
			return connector.getIssue(ctx, command.Input)
		}),
		contracts.File.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.FileInput]) (rpc.File, error) {
			return connector.readFile(ctx, command.Input)
		}),
		backend.BindCapability("github", contracts.Capability),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}
