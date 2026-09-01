package goal

import (
	"fmt"
	"strings"
)

type ReasonCode string

const (
	ReasonNone                   ReasonCode = ""
	ReasonStoppedByUser          ReasonCode = "stoppedByUser"
	ReasonRuntimeRestarted       ReasonCode = "runtimeRestarted"
	ReasonRunStartFailed         ReasonCode = "runStartFailed"
	ReasonAwaitingInput          ReasonCode = "awaitingInput"
	ReasonTerminalOutcomeMissing ReasonCode = "terminalOutcomeMissing"
	ReasonRunNotCompleted        ReasonCode = "runNotCompleted"
	ReasonRunBudgetReached       ReasonCode = "runBudgetReached"
	ReasonCostBudgetReached      ReasonCode = "costBudgetReached"
	ReasonStepBudgetReached      ReasonCode = "stepBudgetReached"
	ReasonBlockedByModel         ReasonCode = "blockedByModel"
)

func (r ReasonCode) Valid() bool {
	switch r {
	case ReasonNone,
		ReasonStoppedByUser,
		ReasonRuntimeRestarted,
		ReasonRunStartFailed,
		ReasonAwaitingInput,
		ReasonTerminalOutcomeMissing,
		ReasonRunNotCompleted,
		ReasonRunBudgetReached,
		ReasonCostBudgetReached,
		ReasonStepBudgetReached,
		ReasonBlockedByModel:
		return true
	default:
		return false
	}
}

// Reason is immutable stopping context. Operational diagnostics never belong
// here; Detail is reserved for a Run outcome or model-authored explanation.
type Reason struct {
	code   ReasonCode
	detail string
}

func (r Reason) Code() ReasonCode { return r.code }
func (r Reason) Detail() string   { return r.detail }
func (r Reason) IsNone() bool     { return r.code == ReasonNone }

func newReason(status Status, code ReasonCode, detail string) (Reason, error) {
	if !code.Valid() || code == ReasonNone {
		return Reason{}, fmt.Errorf("%w: %s reason %q is invalid", ErrInvalid, status, code)
	}
	if detail != strings.TrimSpace(detail) {
		return Reason{}, fmt.Errorf("%w: stop reason detail has surrounding whitespace", ErrInvalid)
	}
	switch status {
	case StatusPaused:
		switch code {
		case ReasonStoppedByUser, ReasonRuntimeRestarted, ReasonRunStartFailed,
			ReasonAwaitingInput, ReasonTerminalOutcomeMissing:
			if detail != "" {
				return Reason{}, fmt.Errorf("%w: reason %q must not carry detail", ErrInvalid, code)
			}
		case ReasonRunNotCompleted:
			if detail == "" {
				return Reason{}, fmt.Errorf("%w: reason %q requires a terminal outcome", ErrInvalid, code)
			}
		default:
			return Reason{}, fmt.Errorf("%w: reason %q cannot pause a Goal", ErrInvalid, code)
		}
	case StatusBlocked:
		switch code {
		case ReasonRunBudgetReached, ReasonCostBudgetReached, ReasonStepBudgetReached:
			if detail != "" {
				return Reason{}, fmt.Errorf("%w: reason %q must not carry detail", ErrInvalid, code)
			}
		case ReasonBlockedByModel:
			if detail == "" {
				return Reason{}, fmt.Errorf("%w: model block requires an explanation", ErrInvalid)
			}
		default:
			return Reason{}, fmt.Errorf("%w: reason %q cannot block a Goal", ErrInvalid, code)
		}
	default:
		return Reason{}, fmt.Errorf("%w: status %q cannot carry a stop reason", ErrInvalid, status)
	}
	return Reason{code: code, detail: detail}, nil
}
