package goal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
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

	ReasonBlockedByModel ReasonCode = "blockedByModel"
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
		ReasonBlockedByModel:
		return true
	default:
		return false
	}
}

// Reason is immutable stopping context. A runNotCompleted pause carries the
// ending of the Run that caused it and a blockedByModel block carries the
// model's explanation; every other reason is its code alone.
type Reason struct {
	code        ReasonCode
	runOutcome  run.Outcome
	explanation string
}

func (r Reason) Code() ReasonCode { return r.code }
func (r Reason) IsNone() bool     { return r.code == ReasonNone }

// RunOutcome reports the ending of the Run that paused a runNotCompleted Goal.
func (r Reason) RunOutcome() (run.Outcome, bool) {
	return r.runOutcome, r.code == ReasonRunNotCompleted
}

// Explanation reports the model's words for a blockedByModel Goal.
func (r Reason) Explanation() (string, bool) {
	return r.explanation, r.code == ReasonBlockedByModel
}

func newReason(status Status, code ReasonCode, runOutcome run.Outcome, explanation string) (Reason, error) {
	if !code.Valid() || code == ReasonNone {
		return Reason{}, fmt.Errorf("%w: %s reason %q is invalid", ErrInvalid, status, code)
	}
	if code != ReasonRunNotCompleted && runOutcome != "" {
		return Reason{}, fmt.Errorf("%w: reason %q must not carry a Run outcome", ErrInvalid, code)
	}
	if code != ReasonBlockedByModel && explanation != "" {
		return Reason{}, fmt.Errorf("%w: reason %q must not carry an explanation", ErrInvalid, code)
	}
	switch status {
	case StatusPaused:
		switch code {
		case ReasonStoppedByUser, ReasonRuntimeRestarted, ReasonRunStartFailed,
			ReasonAwaitingInput, ReasonTerminalOutcomeMissing:
		case ReasonRunNotCompleted:
			if _, known := run.ParseOutcome(string(runOutcome)); !known || runOutcome == run.OutcomeCompleted {
				return Reason{}, fmt.Errorf("%w: reason %q requires an unsuccessful Run outcome", ErrInvalid, code)
			}
		default:
			return Reason{}, fmt.Errorf("%w: reason %q cannot pause a Goal", ErrInvalid, code)
		}
	case StatusBlocked:
		if code != ReasonBlockedByModel {
			return Reason{}, fmt.Errorf("%w: reason %q cannot block a Goal", ErrInvalid, code)
		}
		if explanation == "" {
			return Reason{}, ErrBlockExplanationRequired
		}
		if explanation != strings.TrimSpace(explanation) {
			return Reason{}, fmt.Errorf("%w: block explanation has surrounding whitespace", ErrInvalid)
		}
	default:
		return Reason{}, fmt.Errorf("%w: status %q cannot carry a stop reason", ErrInvalid, status)
	}
	return Reason{code: code, runOutcome: runOutcome, explanation: explanation}, nil
}
