package builtin

import (
	"context"
	"errors"
	"fmt"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

const (
	approveLabel = "Approve"
	rejectLabel  = "Reject"
)

const exitDescription = `Request approval for the current session Plan and exit Plan mode.

Call this only after set_plan contains the complete proposed Plan. This tool
reads that stored Plan; it takes no Plan text or alternatives, so the approved
value cannot differ from the stored session Plan. Approval returns the session to the
default permission mode. Rejection keeps the session in read-only Plan mode.`

type exitArgs struct {
	Revision uint64 `json:"revision" jsonschema:"minimum=1" jsonschema_description:"The exact Plan revision returned by set_plan."`
}

type exiter struct {
	modes planExitPolicy
	plan  planStateReader
}

func newExit(modes planExitPolicy, plan planStateReader, interrupt runs.InterruptFunc) (toolcontract.Tool, error) {
	if modes == nil || plan == nil {
		return nil, nil
	}
	e := &exiter{modes: modes, plan: plan}
	return newInterruptTool[exitArgs](
		toolcontract.FuncConfig{Name: string(builtincontract.ExitPlanMode), Description: exitDescription},
		interrupt,
		e.request,
		e.render,
	)
}

func (e *exiter) request(ctx context.Context, args exitArgs) (runs.Interrupt, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return nil, errors.New("exit_plan_mode: no active session")
	}
	mode, err := e.modes.Mode(ctx, sessionID)
	if err != nil {
		return nil, pluginsdk.DefiniteToolFailure(fmt.Errorf("exit_plan_mode: %w", err))
	}
	// Calling this outside Plan mode, or with nothing planned, is the model
	// mis-sequencing its own tools. Both messages are written for it, so they have
	// to arrive as this call's outcome rather than end the Run.
	if mode != approval.ModePlan {
		return nil, pluginsdk.DefiniteToolFailure(errors.New("exit_plan_mode: current session is not in Plan mode"))
	}
	state, err := e.plan.State(ctx, sessionID)
	if err != nil {
		return nil, pluginsdk.DefiniteToolFailure(fmt.Errorf("exit_plan_mode: %w", err))
	}
	steps := state.Steps()
	if len(steps) == 0 {
		return nil, pluginsdk.DefiniteToolFailure(errors.New("exit_plan_mode: current Plan is empty; call set_plan before requesting approval"))
	}

	committed, ok := state.State()
	if !ok || committed.Revision() != args.Revision {
		return nil, pluginsdk.DefiniteToolFailure(errors.New("exit_plan_mode: proposed revision is no longer current"))
	}
	prompt, err := e.plan.Review(ctx, committed)
	if err != nil {
		return nil, pluginsdk.DefiniteToolFailure(err)
	}
	arguments := fmt.Sprintf(`{"revision":%d}`, args.Revision)
	pending := runs.QuestionPrompt{
		ToolName:  string(builtincontract.ExitPlanMode),
		Arguments: arguments,
		Fields: []runs.QuestionFieldSpec{{
			Prompt: prompt,
			Header: "Plan",
			Options: []runs.QuestionOptionSpec{
				{Label: approveLabel, Description: "Approve this Plan and allow execution"},
				{Label: rejectLabel, Description: "Keep Plan mode and revise the Plan"},
			},
		}},
	}
	if validateErr := pending.Validate(); validateErr != nil {
		return nil, fmt.Errorf("exit_plan_mode: %w", validateErr)
	}
	return pending, nil
}

func (e *exiter) render(ctx context.Context, args exitArgs, resolution run.InterruptResolution) (string, error) {
	if selectedChoice(resolution.Question().Answers()) != approveLabel {
		return "Plan not approved. The session remains in Plan mode; revise the Plan or continue investigating.", nil
	}
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "", errors.New("exit_plan_mode: no active session")
	}
	restored, changed, err := e.modes.ExitPlanMode(ctx, sessionID, args.Revision)
	if err != nil {
		return "", err
	}
	if !changed {
		return "", pluginsdk.DefiniteToolFailure(errors.New("exit_plan_mode: the approved version or Plan mode is no longer current"))
	}
	return fmt.Sprintf("Plan approved. Plan mode exited; the session follows the default permission mode %s. Execute the Plan.", modeName(restored)), nil
}

func selectedChoice(answers [][]string) string {
	if len(answers) == 0 || len(answers[0]) == 0 {
		return ""
	}
	return answers[0][0]
}

func modeName(mode approval.Mode) string {
	switch mode {
	case approval.ModeSafe:
		return "safe"
	case approval.ModeBalanced:
		return "balanced"
	case approval.ModeYolo:
		return "yolo"
	default:
		return "unknown"
	}
}
