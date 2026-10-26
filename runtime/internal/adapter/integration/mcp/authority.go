package mcp

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	chat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

type sourceRegistry interface {
	Dispatchable(context.Context, mcpserver.ID) (mcpserver.Server, bool, error)
	Connection(context.Context, mcpserver.ID) (mcpapp.Launch, error)
}

// Captured tools cannot retain authority after their source or session is withdrawn.
type authorizedTool struct {
	toolcontract.Tool
	registry sourceRegistry
	source   Executable
	config   ServerConfig
	prepare  ToolPreparation
	host     ToolHost
}

func (t authorizedTool) Unwrap() toolcontract.Tool { return t.Tool }
func (t authorizedTool) Call(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
	if err := t.admit(ctx); err != nil {
		return toolcontract.Result{}, err
	}
	if installation, managed := t.config.Server.Source.Origin().Installation(); managed {
		captured, err := operation.Capture(ctx, extension.Target{Scope: extension.ScopeInstallation, Reference: installation.String()})
		if err != nil {
			return toolcontract.Result{}, err
		}
		metadata := scopemcp.RequestMetaFromContext(ctx)
		if err := metadata.Set(pluginsdk.InvocationMetadataKey, captured); err != nil {
			return toolcontract.Result{}, err
		}
		ctx = scopemcp.WithRequestMeta(ctx, metadata)
		ctx, err = pluginsdk.WithInvocation(ctx, captured)
		if err != nil {
			return toolcontract.Result{}, err
		}
	}
	if t.prepare == nil {
		return t.Tool.Call(ctx, invocation)
	}
	call, release, err := t.prepare(ctx, t.source, t.host)
	if err != nil {
		return toolcontract.Result{}, err
	}
	if release == nil {
		return toolcontract.Result{}, errors.New("mcp: prepared tool call requires release")
	}
	defer release()
	if call == nil {
		return toolcontract.Result{}, errors.New("mcp: prepared tool call is required")
	}
	if err := t.admit(ctx); err != nil {
		return toolcontract.Result{}, err
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

func (p *Pool) authorizedTools(catalog []Executable) []toolcontract.Tool {
	result := make([]toolcontract.Tool, 0, len(catalog))
	for _, executable := range catalog {
		result = append(result, authorizedTool{Tool: executable, registry: p.registry, source: executable, config: executable.SourceConfig(), prepare: p.prepare, host: p.toolHost(executable)})
	}
	return result
}
