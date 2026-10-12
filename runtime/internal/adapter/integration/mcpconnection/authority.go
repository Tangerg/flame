package mcpconnection

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	chat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type sourceRegistry interface {
	Dispatchable(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)
	Connection(context.Context, mcpserver.ID) (mcpapp.Launch, error)
	Service(context.Context, mcpserver.ID) (plugin.ServiceKind, bool, error)
}

// Captured tools cannot retain authority after their source or session is withdrawn.
type authorizedTool struct {
	toolcontract.Tool
	registry      sourceRegistry
	source        mcp.Executable
	config        mcp.ServerConfig
	memoryContext MemoryContext
	publish       invalidation.Publish
}

func (t authorizedTool) Unwrap() toolcontract.Tool { return t.Tool }
func (t authorizedTool) Call(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
	current, found, err := t.registry.Dispatchable(ctx, t.config.ID())
	if err != nil {
		failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindFailed, Cause: err, Output: chat.NewTextToolOutput("current source authority could not be verified")})
		if failureErr != nil {
			return chat.ToolOutput{}, failureErr
		}
		return chat.ToolOutput{}, failure
	}
	if !found || !current.Enabled {
		return chat.ToolOutput{}, rejected("source authority is no longer admitted")
	}
	if err := current.Validate(); err != nil {
		failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindFailed, Cause: err, Output: chat.NewTextToolOutput("current source configuration could not be verified")})
		if failureErr != nil {
			return chat.ToolOutput{}, failureErr
		}
		return chat.ToolOutput{}, failure
	}
	if !t.config.Server.SameConnection(current) {
		return chat.ToolOutput{}, rejected("source configuration is no longer current")
	}
	if !t.source.Current() {
		return chat.ToolOutput{}, rejected("source connection is no longer current")
	}
	kind, managed, err := t.registry.Service(ctx, current.ID())
	if err != nil {
		return chat.ToolOutput{}, err
	}
	if managed && kind == plugin.MemoryService {
		if t.memoryContext == nil {
			return chat.ToolOutput{}, errors.New("memory: model tool context is unavailable")
		}
		scoped, release, err := t.memoryContext(ctx, t.source)
		if err != nil {
			return chat.ToolOutput{}, err
		}
		defer release()
		ctx = scoped
	}
	if managed && kind == plugin.SchedulesService {
		// A backend may commit before its reply is lost. This notice only
		// invalidates reads; the backend remains the sole source of facts.
		defer t.publish.Notify(invalidation.ForSchedules())
	}
	return t.Tool.Call(ctx, invocation)
}

// rejected is the definite refusal of a call whose admitted authority or
// connection was withdrawn; nothing reached the remote server.
func rejected(reason string) error {
	failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindRejected, Output: chat.NewTextToolOutput(reason)})
	if err != nil {
		return err
	}
	return failure
}

func (p *Pool) authorizedTools(catalog []mcp.Executable) []toolcontract.Tool {
	result := make([]toolcontract.Tool, 0, len(catalog))
	for _, executable := range catalog {
		result = append(result, authorizedTool{Tool: executable, registry: p.registry, source: executable, config: executable.SourceConfig(), memoryContext: p.memoryContext, publish: p.publish})
	}
	return result
}
