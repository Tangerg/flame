package approval

import "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

// StandingDecision is a remembered approval rule matched for this call.
type StandingDecision struct {
	Decision Decision
	Matched  bool
}

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
	Action      GateAction
	Denial      DenialCause
	SafetyClass tool.SafetyClass
	Risk        tool.RiskLevel
	PromptCause PromptCause
}

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
	PromptCauseNone                PromptCause = ""
	PromptCauseNonMutating         PromptCause = "nonMutating"
	PromptCauseWorkspaceWrite      PromptCause = "workspaceWrite"
	PromptCauseWorkspaceCommand    PromptCause = "workspaceCommand"
	PromptCauseNetworkAccess       PromptCause = "networkAccess"
	PromptCauseUnknownSafety       PromptCause = "unknownSafety"
	PromptCauseOutsideWorkspace    PromptCause = "outsideWorkspace"
	PromptCauseUnknownMutation     PromptCause = "unknownMutation"
	PromptCauseCatastrophicCommand PromptCause = "catastrophicCommand"
)

// Plan applies hook and approval-mode policy to one tool call. It does not
// read remembered rules and it does not trigger HITL; callers only do those
// side effects when the returned plan asks for [GatePrompt].
func (t ToolCallInput) Plan() ToolCallPlan {
	plan := ToolCallPlan{Action: GatePass, SafetyClass: t.SafetyClass}
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
	plan.Action = action
	switch action {
	case GateDeny:
		plan.Denial = DenialPlanMode
	case GatePrompt:
		plan.Risk = t.SafetyClass.Risk()
		plan.PromptCause = promptCauseForSafetyClass(t.SafetyClass)
		if immunity != tool.BypassAllowed {
			plan.Risk = tool.RiskHigh
			plan.PromptCause = promptCauseForBypassImmunity(immunity)
		}
	}
	return plan
}

// ResolvePromptShortcuts applies non-HITL prompt short-circuits: remembered
// rules first, then an explicit auto-approve grant. It is a no-op unless the
// plan is [GatePrompt].
func (t ToolCallPlan) ResolvePromptShortcuts(standing StandingDecision, autoApproved bool) ToolCallPlan {
	if t.Action != GatePrompt {
		return t
	}
	if standing.Matched {
		if standing.Decision == Deny {
			t.Action = GateDeny
			t.Denial = DenialRememberedRule
			return t
		}
		t.Action = GatePass
		return t
	}
	if autoApproved {
		t.Action = GatePass
	}
	return t
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
		return PromptCauseUnknownSafety
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
		return PromptCauseNone
	}
}
