package pluginservices

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/samber/lo"
)

type toolCallers interface {
	ToolCaller(context.Context, string) (plugin.ToolCaller, bool, error)
}

type CallerRestorer struct {
	commands *plugins.Commands
	tools    toolCallers
}

func NewCallerRestorer(commands *plugins.Commands, tools toolCallers) (*CallerRestorer, error) {
	if commands == nil || lo.IsNil(tools) {
		return nil, errors.New("plugin callers: command and Tool routing are required")
	}
	return &CallerRestorer{commands: commands, tools: tools}, nil
}

func (r *CallerRestorer) RestoreInvocation(ctx context.Context, invocation portable.Invocation) (context.Context, error) {
	if err := extension.ValidateInvocation(invocation); err != nil {
		return nil, err
	}
	caller, found, err := r.tools.ToolCaller(ctx, invocation.ID)
	if err != nil {
		return nil, err
	}
	if !found {
		return r.commands.RestoreInvocation(ctx, invocation)
	}
	actor, managed := operation.ActorFrom(ctx)
	if !managed || !caller.Matches(invocation, actor.InstallationID(), actor.Release(), actor.Authority(), actor.Server()) {
		return nil, plugin.ErrCommandConflict
	}
	return operation.Adopt(ctx, invocation.ID, invocation.Origin, invocation.Parent), nil
}
