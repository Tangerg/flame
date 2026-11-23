package builtin

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	plandomain "github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type planStateReader interface {
	State(ctx context.Context, sessionID identity.SessionID) (plandomain.Current, error)
	Review(context.Context, plandomain.State) (string, error)
}

type planReplacer interface {
	Replace(ctx context.Context, sessionID identity.SessionID, steps []plandomain.Step) (plandomain.State, error)
}

type PlanUseCases interface {
	planStateReader
	planReplacer
}

type planEnterPolicy interface {
	EnterPlanMode(ctx context.Context, sessionID identity.SessionID) (changed bool, err error)
}

type planExitPolicy interface {
	Mode(ctx context.Context, sessionID identity.SessionID) (approval.Mode, error)
	ExitPlanMode(ctx context.Context, sessionID identity.SessionID, revision uint64) (restored approval.Mode, changed bool, err error)
}

type PlanModePolicy interface {
	planEnterPolicy
	planExitPolicy
}

type PlanTools struct {
	Enter toolcontract.Tool
	Set   toolcontract.Tool
	Exit  toolcontract.Tool
}

func BuildPlan(modes PlanModePolicy, plans PlanUseCases, available func(context.Context) (bool, error), interrupt runs.InterruptFunc) (PlanTools, error) {
	enter, err := newEnter(modes, available)
	if err != nil {
		return PlanTools{}, fmt.Errorf("plan: build enter_plan_mode: %w", err)
	}
	set, err := newSet(plans)
	if err != nil {
		return PlanTools{}, fmt.Errorf("plan: build set_plan: %w", err)
	}
	exit, err := newExit(modes, plans, interrupt)
	if err != nil {
		return PlanTools{}, fmt.Errorf("plan: build exit_plan_mode: %w", err)
	}
	return PlanTools{Enter: enter, Set: set, Exit: exit}, nil
}
