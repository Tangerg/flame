package approval

import "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

type GateAction string

const (
	GatePass   GateAction = "pass"
	GatePrompt GateAction = "prompt"
	GateDeny   GateAction = "deny"
)

func GateFor(cls tool.SafetyClass, mode Mode) GateAction {
	if mode == ModeYolo || cls == tool.SafetyClassSafe {
		return GatePass
	}
	switch mode {
	case ModePlan:
		return GateDeny
	case ModeSafe:
		return GatePrompt
	case ModeBalanced:
		switch cls {
		case tool.SafetyClassWrite, tool.SafetyClassNetwork:
			return GatePass
		default:
			// Exec and unknown classes fail closed. In particular, the invalid
			// zero value must not inherit write's balanced-mode exemption.
			return GatePrompt
		}
	default:
		// An unknown mode must not silently disable approval for a side-effecting
		// tool, even when this pure function runs without mode validation.
		return GatePrompt
	}
}
