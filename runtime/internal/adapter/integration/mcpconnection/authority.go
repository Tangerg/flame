package mcpconnection

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	chat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type sourceRegistry interface {
	Get(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error)
	Connection(context.Context, mcpserver.ServerName) (mcpserver.Server, error)
}
type authorizedTool struct {
	toolcontract.Tool
	registry sourceRegistry
	config   mcp.ServerConfig
}

func (t authorizedTool) Unwrap() toolcontract.Tool { return t.Tool }
func (t authorizedTool) Call(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
	current, found, err := t.registry.Get(ctx, t.config.Name)
	if err != nil {
		failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindFailed, Cause: err, Output: chat.NewTextToolOutput("current source authority could not be verified")})
		if failureErr != nil {
			return chat.ToolOutput{}, failureErr
		}
		return chat.ToolOutput{}, failure
	}
	if !found || !current.Enabled {
		failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindRejected, Output: chat.NewTextToolOutput("source authority is no longer admitted")})
		if err != nil {
			return chat.ToolOutput{}, err
		}
		return chat.ToolOutput{}, failure
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
		failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindRejected, Output: chat.NewTextToolOutput("source configuration is no longer current")})
		if err != nil {
			return chat.ToolOutput{}, err
		}
		return chat.ToolOutput{}, failure
	}
	return t.Tool.Call(ctx, invocation)
}
func (p *Pool) authorizedTools(catalog []mcp.Executable) []toolcontract.Tool {
	result := make([]toolcontract.Tool, 0, len(catalog))
	for _, executable := range catalog {
		result = append(result, authorizedTool{Tool: executable, registry: p.registry, config: executable.SourceConfig()})
	}
	return result
}
