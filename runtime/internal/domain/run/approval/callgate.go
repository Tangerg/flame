package approval

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// ToolCallInput is the pure policy input for one tool call. The arguments
// themselves are not policy input: this gate decides from the call's safety
// class, the scope of the file mutation it declares, and the shell command it
// would run.
type ToolCallInput struct {
	Mode Mode
	// RequireApproval is a PreToolUse hook's escalation: force a prompt for a
	// call the mode would otherwise pass. The hook's own block and rewrite
	// decisions are applied at the Tool boundary that runs it, not here.
	RequireApproval bool
	SafetyClass     tool.SafetyClass
	FileMutation    tool.FileMutationScope
	ShellCommand    string
}

// ToolCallPlan is the approval policy's verdict before any HITL interrupt is
// executed: pass, deny, or prompt. The call payload stays with the caller that
// supplied it — this gate never rewrites arguments.
type ToolCallPlan struct {
	action      GateAction
	denial      DenialCause
	safetyClass tool.SafetyClass
	risk        tool.RiskLevel
	promptCause PromptCause
}

func (t ToolCallPlan) Action() GateAction            { return t.action }
func (t ToolCallPlan) Denial() DenialCause           { return t.denial }
func (t ToolCallPlan) SafetyClass() tool.SafetyClass { return t.safetyClass }
func (t ToolCallPlan) Risk() tool.RiskLevel          { return t.risk }
func (t ToolCallPlan) PromptCause() PromptCause      { return t.promptCause }

// DenialCause is the policy source of a refusal. The wording a denied call
// reports belongs to the caller that presents it.
type DenialCause string

const (
	DenialPlanMode       DenialCause = "planMode"
	DenialRememberedRule DenialCause = "rememberedRule"
)

// PromptCause is the policy fact an approval surface explains to a user.
type PromptCause string

const (
	promptCauseNone                PromptCause = ""
	PromptCauseNonMutating         PromptCause = "nonMutating"
	PromptCauseWorkspaceWrite      PromptCause = "workspaceWrite"
	PromptCauseWorkspaceCommand    PromptCause = "workspaceCommand"
	PromptCauseNetworkAccess       PromptCause = "networkAccess"
	promptCauseUnknownSafety       PromptCause = "unknownSafety"
	PromptCauseOutsideWorkspace    PromptCause = "outsideWorkspace"
	PromptCauseUnknownMutation     PromptCause = "unknownMutation"
	PromptCauseCatastrophicCommand PromptCause = "catastrophicCommand"
)

func (t ToolCallInput) Plan() ToolCallPlan {
	plan := ToolCallPlan{safetyClass: t.SafetyClass}
	action := GateFor(t.SafetyClass, t.Mode)
	// Bypass-immune escalation: a call dangerous enough (a mutation escaping the
	// workspace, or a high-confidence catastrophic shell command) is confirmed
	// even under a mode that would auto-pass it (Yolo, or Balanced for
	// file mutations). This override is not defeated by "approve everything" — the
	// same seam a PreToolUse hook's Ask uses to force a prompt, but
	// tool/argument-driven and built in. A remembered approval still lets a repeat
	// call through.
	immunity := tool.BypassImmunityFor(t.FileMutation, t.ShellCommand)
	if action == GatePass && (t.RequireApproval || immunity != tool.BypassAllowed) {
		action = GatePrompt
	}
	plan.action = action
	switch action {
	case GateDeny:
		plan.denial = DenialPlanMode
	case GatePrompt:
		plan.risk = t.SafetyClass.Risk()
		plan.promptCause = promptCauseForSafetyClass(t.SafetyClass)
		if immunity != tool.BypassAllowed {
			plan.risk = tool.RiskHigh
			plan.promptCause = promptCauseForBypassImmunity(immunity)
		}
	}
	return plan
}

// A saved denial survives a switch to automatic approval. A saved allowance
// cannot reopen a call that the session's read-only stance has already refused.
func (t ToolCallPlan) ApplyRule(decision Decision) (ToolCallPlan, error) {
	if !decision.Valid() {
		return ToolCallPlan{}, fmt.Errorf("%w: unknown decision %q", ErrInvalidRule, decision)
	}
	switch t.action {
	case GateDeny:
		return t, nil
	case GatePass, GatePrompt:
	default:
		return ToolCallPlan{}, fmt.Errorf("approval: unknown gate action %q", t.action)
	}
	if decision == Deny {
		return ToolCallPlan{action: GateDeny, denial: DenialRememberedRule, safetyClass: t.safetyClass}, nil
	}
	return ToolCallPlan{action: GatePass, safetyClass: t.safetyClass}, nil
}

// DecisionOf maps an approve/deny boolean to the approval domain's verdict.
func DecisionOf(approved bool) Decision {
	if approved {
		return Allow
	}
	return Deny
}

func promptCauseForSafetyClass(class tool.SafetyClass) PromptCause {
	switch class {
	case tool.SafetyClassSafe:
		return PromptCauseNonMutating
	case tool.SafetyClassWrite:
		return PromptCauseWorkspaceWrite
	case tool.SafetyClassExec:
		return PromptCauseWorkspaceCommand
	case tool.SafetyClassNetwork:
		return PromptCauseNetworkAccess
	default:
		return promptCauseUnknownSafety
	}
}

func promptCauseForBypassImmunity(immunity tool.BypassImmunity) PromptCause {
	switch immunity {
	case tool.BypassImmuneOutsideWorkspace:
		return PromptCauseOutsideWorkspace
	case tool.BypassImmuneUnknownMutation:
		return PromptCauseUnknownMutation
	case tool.BypassImmuneCatastrophicCommand:
		return PromptCauseCatastrophicCommand
	default:
		return promptCauseNone
	}
}
