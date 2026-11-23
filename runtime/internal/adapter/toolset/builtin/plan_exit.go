package builtin

import (
	"context"
	"fmt"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/samber/lo"

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
	if lo.IsNil(modes) || lo.IsNil(plan) {
		return nil, fmt.Errorf("%s: Plan mode policy and Plan state are required", builtincontract.ExitPlanMode)
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
	if sessionID.IsZero() {
		return nil, fmt.Errorf("%s: no active session", builtincontract.ExitPlanMode)
	}
	mode, err := e.modes.Mode(ctx, sessionID)
	if err != nil {
		return nil, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: %w", builtincontract.ExitPlanMode, err))
	}
	if mode != approval.ModePlan {
		return nil, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: current session is not in Plan mode", builtincontract.ExitPlanMode))
	}
	state, err := e.plan.State(ctx, sessionID)
	if err != nil {
		return nil, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: %w", builtincontract.ExitPlanMode, err))
	}
	steps := state.Steps()
	if len(steps) == 0 {
		return nil, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: current Plan is empty; call set_plan before requesting approval", builtincontract.ExitPlanMode))
	}

	committed, ok := state.State()
	if !ok || committed.Revision() != args.Revision {
		return nil, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: proposed revision is no longer current", builtincontract.ExitPlanMode))
	}
	prompt, err := e.plan.Review(ctx, committed)
	if err != nil {
		return nil, pluginsdk.DefiniteToolFailure(err)
	}
	arguments := fmt.Sprintf(`{"revision":%d}`, args.Revision)
	pending := runs.QuestionPrompt{
		ToolName:  string(builtincontract.ExitPlanMode),
		Arguments: arguments,
		Fields: []transcript.QuestionField{{
			Prompt: prompt,
			Header: "Plan",
			Kind:   questioncontract.Choice,
			Options: []transcript.QuestionOption{
				{Label: approveLabel, Description: "Approve this Plan and allow execution"},
				{Label: rejectLabel, Description: "Keep Plan mode and revise the Plan"},
			},
		}},
	}
	if validateErr := pending.Validate(); validateErr != nil {
		return nil, fmt.Errorf("%s: %w", builtincontract.ExitPlanMode, validateErr)
	}
	return pending, nil
}

func (e *exiter) render(ctx context.Context, args exitArgs, resolution run.InterruptResolution) (string, error) {
	if selectedChoice(resolution.Question().Answers()) != approveLabel {
		return "Plan not approved. The session remains in Plan mode; revise the Plan or continue investigating.", nil
	}
	sessionID := executionctx.SessionID(ctx)
	if sessionID.IsZero() {
		return "", fmt.Errorf("%s: no active session", builtincontract.ExitPlanMode)
	}
	restored, changed, err := e.modes.ExitPlanMode(ctx, sessionID, args.Revision)
	if err != nil {
		return "", err
	}
	if !changed {
		return "", pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: the approved version or Plan mode is no longer current", builtincontract.ExitPlanMode))
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
