package approval

import "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

// GateAction is the three-way outcome of the per-call permission gate: run it,
// ask the user, or refuse outright. Replacing the older bool ("needs approval?")
// lets ModePlan express "deny without prompting" — a stance a bool can't capture.
type GateAction string

const (
	// GatePass runs the tool without prompting.
	GatePass GateAction = "pass"
	// GatePrompt registers an approval request and waits for a human decision.
	GatePrompt GateAction = "prompt"
	// GateDeny refuses outright with no prompt (ModePlan's read-only stance).
	GateDeny GateAction = "deny"
)

// GateFor encodes the (tool-class, mode) → gate action. The rules mirror the
// strictness gradient documented on [Mode]:
//
//   - ModeYolo     → always pass
//   - ModeBalanced → auto-allow known write/network; prompt on exec and unknown
//   - ModeSafe     → prompt on every write / network / exec / unknown tool
//   - ModePlan     → deny every write / network / exec / unknown tool (read-only)
//
// This is only the mode's default. ToolCallInput and ToolCallPlan own escalation
// and saved rules, so read-only calls can still be refused by an explicit rule.
func GateFor(cls tool.SafetyClass, mode Mode) GateAction {
	if mode == ModeYolo {
		return GatePass
	}
	if cls == tool.SafetyClassSafe {
		return GatePass
	}
	switch mode {
	case ModePlan:
		return GateDeny
	case ModeSafe:
		return GatePrompt // every non-safe class, including unknown
	case ModeBalanced:
		switch cls {
		case tool.SafetyClassWrite, tool.SafetyClassNetwork:
			return GatePass
		default:
			// Exec and unknown classes fail closed. In particular, the invalid
			// zero value must not inherit write's balanced-mode exemption.
			return GatePrompt
		}
	}
	// An unknown policy mode must not silently disable approval for a
	// side-effecting tool. Mode validation normally rejects this earlier; this
	// fallback keeps the pure policy function safe in isolation too.
	return GatePrompt
}
