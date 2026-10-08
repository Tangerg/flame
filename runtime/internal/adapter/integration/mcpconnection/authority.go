package mcpconnection

import (
	"context"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	chat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type sourceRegistry interface {
	Dispatchable(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)
	Connection(context.Context, mcpserver.ID) (mcpapp.Launch, error)
}

// authorizedTool rechecks revocable authority at dispatch (draft §7.5): the
// source must still be admitted, with the same connection configuration, and
// the session that admitted this executable must still be live.
type authorizedTool struct {
	toolcontract.Tool
	registry sourceRegistry
	source   mcp.Executable
	config   mcp.ServerConfig
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
	config, err := configFromServer(current)
	if err != nil {
		failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindFailed, Cause: err, Output: chat.NewTextToolOutput("current source configuration could not be verified")})
		if failureErr != nil {
			return chat.ToolOutput{}, failureErr
		}
		return chat.ToolOutput{}, failure
	}
	if !t.config.SameConnection(config) {
		return chat.ToolOutput{}, rejected("source configuration is no longer current")
	}
	if !t.source.Current() {
		return chat.ToolOutput{}, rejected("source connection is no longer current")
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
		result = append(result, authorizedTool{Tool: executable, registry: p.registry, source: executable, config: executable.SourceConfig()})
	}
	return result
}
