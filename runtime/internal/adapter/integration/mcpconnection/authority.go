package mcpconnection

import (
	"context"
	"errors"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/cancellation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	chat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type sourceRegistry interface {
	Dispatchable(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)
	Connection(context.Context, mcpserver.ID) (mcpapp.Launch, error)
}

// Captured tools cannot retain authority after their source or session is withdrawn.
type authorizedTool struct {
	toolcontract.Tool
	registry sourceRegistry
	source   mcp.Executable
	config   mcp.ServerConfig
	prepare  ToolPreparation
}

func (t authorizedTool) Unwrap() toolcontract.Tool { return t.Tool }
func (t authorizedTool) Call(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
	if err := t.admit(ctx); err != nil {
		return chat.ToolOutput{}, err
	}
	if t.prepare == nil {
		return t.Tool.Call(ctx, invocation)
	}
	call, release, err := t.prepare(ctx, t.source)
	if err != nil {
		return chat.ToolOutput{}, err
	}
	if release == nil {
		return chat.ToolOutput{}, errors.New("mcp: prepared tool call requires release")
	}
	defer release()
	if call == nil {
		return chat.ToolOutput{}, errors.New("mcp: prepared tool call is required")
	}
	if err := t.admit(ctx); err != nil {
		return chat.ToolOutput{}, err
	}
	return call(invocation)
}

func (t authorizedTool) admit(ctx context.Context) error {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	current, found, err := t.registry.Dispatchable(ctx, t.config.ID())
	if cause := cancellation.Err(ctx); cause != nil {
		return errors.Join(err, cause)
	}
	if err != nil {
		failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindFailed, Cause: err, Output: chat.NewTextToolOutput("current source authority could not be verified")})
		if failureErr != nil {
			return failureErr
		}
		return failure
	}
	if !found || !current.Enabled {
		return rejected("source authority is no longer admitted")
	}
	if err := current.Validate(); err != nil {
		failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindFailed, Cause: err, Output: chat.NewTextToolOutput("current source configuration could not be verified")})
		if failureErr != nil {
			return failureErr
		}
		return failure
	}
	if !t.config.Server.SameConnection(current) {
		return rejected("source configuration is no longer current")
	}
	if !t.source.Current() {
		return rejected("source connection is no longer current")
	}
	return nil
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
		result = append(result, authorizedTool{Tool: executable, registry: p.registry, source: executable, config: executable.SourceConfig(), prepare: p.prepare})
	}
	return result
}
