package approval

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestToolCallInputPlan_HookAskEscalatesPass(t *testing.T) {
	plan := ToolCallInput{
		Mode:            ModeBalanced,
		RequireApproval: true,
		SafetyClass:     tool.SafetyClassWrite,
		FileMutation:    tool.FileMutationNone,
	}.Plan()
	if plan.Action() != GatePrompt || plan.SafetyClass() != tool.SafetyClassWrite || plan.Risk() == "" || plan.PromptCause() != PromptCauseWorkspaceWrite {
		t.Fatalf("plan = %+v, want prompt for hook ask", plan)
	}
}

func TestToolCallInputPlan_OutOfWorkspaceMutationIsBypassImmune(t *testing.T) {
	// Yolo would auto-pass a write, but a target outside the workspace must still
	// be confirmed — shown as high risk with a specific reason.
	plan := ToolCallInput{
		Mode:         ModeYolo,
		SafetyClass:  tool.SafetyClassWrite,
		FileMutation: tool.FileMutationOutsideWorkspace,
	}.Plan()
	if plan.Action() != GatePrompt {
		t.Fatalf("action = %v, want GatePrompt (bypass-immune)", plan.Action())
	}
	if plan.Risk() != tool.RiskHigh || plan.PromptCause() != PromptCauseOutsideWorkspace {
		t.Fatalf("plan = %+v, want high risk + out-of-workspace reason", plan)
	}
}

func TestToolCallInputPlan_InWorkspaceMutationStillAutoPassesInYolo(t *testing.T) {
	plan := ToolCallInput{
		Mode:         ModeYolo,
		SafetyClass:  tool.SafetyClassWrite,
		FileMutation: tool.FileMutationWithinWorkspace,
	}.Plan()
	if plan.Action() != GatePass {
		t.Fatalf("action = %v, want GatePass (in-workspace write under Yolo)", plan.Action())
	}
}

func TestToolCallInputPlan_NoMutationScopeDoesNotEscalate(t *testing.T) {
	plan := ToolCallInput{
		Mode:         ModeYolo,
		SafetyClass:  tool.SafetyClassWrite,
		FileMutation: tool.FileMutationNone,
	}.Plan()
	if plan.Action() != GatePass {
		t.Fatalf("action = %v, want GatePass (no workspace configured)", plan.Action())
	}
}

func TestToolCallInputPlan_ModePlanDenyBeatsHookAsk(t *testing.T) {
	plan := ToolCallInput{
		Mode:            ModePlan,
		RequireApproval: true,
		SafetyClass:     tool.SafetyClassExec,
		FileMutation:    tool.FileMutationNone,
	}.Plan()
	if plan.Action() != GateDeny || plan.Denial() != DenialPlanMode {
		t.Fatalf("plan = %+v, want plan-mode deny", plan)
	}
}

func TestToolCallPlanDenialSurvivesAutomaticApproval(t *testing.T) {
	for _, mode := range []Mode{ModeSafe, ModeBalanced, ModeYolo, ModePlan} {
		for _, class := range []tool.SafetyClass{tool.SafetyClassSafe, tool.SafetyClassWrite, tool.SafetyClassExec, tool.SafetyClassNetwork} {
			t.Run(string(mode)+"/"+string(class), func(t *testing.T) {
				plan := ToolCallInput{Mode: mode, SafetyClass: class, FileMutation: tool.FileMutationNone}.Plan()
				resolved, err := plan.ApplyRule(Deny)
				if err != nil || resolved.Action() != GateDeny {
					t.Fatalf("saved denial = %+v, %v", resolved, err)
				}
				wantCause := DenialRememberedRule
				if plan.Action() == GateDeny {
					wantCause = DenialPlanMode
				}
				if resolved.Denial() != wantCause || resolved.PromptCause() != promptCauseNone || resolved.Risk() != "" {
					t.Fatalf("resolved denial carries inconsistent causes: %+v", resolved)
				}
			})
		}
	}
}

func TestToolCallPlanAllowanceCannotOverrideReadOnlyMode(t *testing.T) {
	for _, mode := range []Mode{ModeSafe, ModePlan} {
		plan := ToolCallInput{Mode: mode, SafetyClass: tool.SafetyClassExec, FileMutation: tool.FileMutationNone}.Plan()
		resolved, err := plan.ApplyRule(Allow)
		if err != nil {
			t.Fatal(err)
		}
		if mode == ModePlan {
			if resolved != plan {
				t.Fatalf("saved allowance reopened Plan-mode refusal: %+v", resolved)
			}
			continue
		}
		if resolved.Action() != GatePass || resolved.PromptCause() != promptCauseNone || resolved.Risk() != "" {
			t.Fatalf("saved allowance left a prompt: %+v", resolved)
		}
	}
}

func TestToolCallPlanRejectsInvalidRuleDecision(t *testing.T) {
	plan := ToolCallInput{Mode: ModeBalanced, SafetyClass: tool.SafetyClassWrite, FileMutation: tool.FileMutationNone}.Plan()
	if _, err := plan.ApplyRule(Decision("invalid")); !errors.Is(err, ErrInvalidRule) {
		t.Fatalf("ApplyRule error = %v, want ErrInvalidRule", err)
	}
}
