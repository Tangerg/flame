package approval

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type ToolCallInput struct {
	Mode            Mode
	RequireApproval bool
	SafetyClass     tool.SafetyClass
	FileMutation    tool.FileMutationScope
	ShellCommand    string
}

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

type DenialCause string

const (
	DenialPlanMode       DenialCause = "planMode"
	DenialRememberedRule DenialCause = "rememberedRule"
)

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
	// Bypass-immune escalation: a mutation escaping the workspace or a high-confidence
	// catastrophic shell command is confirmed even under an auto-pass mode; a
	// remembered approval still lets a repeat through.
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
	case GatePass:
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
