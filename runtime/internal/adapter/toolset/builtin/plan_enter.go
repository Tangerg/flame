package builtin

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

const enterDescription = `Enter read-only Plan mode for the current session.

Use this when the user asks you to investigate, design, or propose a Plan before
making changes. It blocks write, command, and network tools only for this
session; leaving Plan mode returns it to the default permission mode.
It does not create or modify the Plan; use set_plan after entering. Entering
requires no approval because it only reduces permissions.`

type enterArgs struct{}

type enterer struct {
	modes     planEnterPolicy
	available func(context.Context) (bool, error)
}

func newEnter(modes planEnterPolicy, available func(context.Context) (bool, error)) (toolcontract.Tool, error) {
	if modes == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[enterArgs, string](
		toolcontract.FuncConfig{Name: string(builtincontract.EnterPlanMode), Description: enterDescription},
		(&enterer{modes: modes, available: available}).enter,
	)
}

func (e *enterer) enter(ctx context.Context, _ enterArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "", errors.New("enter_plan_mode: no active session")
	}
	if e.available != nil {
		ready, err := e.available(ctx)
		if err != nil {
			return "", err
		}
		if !ready {
			return "", pluginsdk.DefiniteToolFailure(plugin.ErrUnavailable)
		}
	}
	changed, err := e.modes.EnterPlanMode(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !changed {
		return "The session is already in Plan mode. Continue investigating or update the Plan with set_plan.", nil
	}
	return "Plan mode entered for this session. Write, command, and network tools are now blocked. Investigate with read-only tools and maintain the proposed Plan with set_plan.", nil
}
