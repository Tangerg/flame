package goal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type ReasonCode string

const (
	ReasonNone                   ReasonCode = ""
	ReasonStoppedByUser          ReasonCode = ReasonCode(goalworkflow.GoalReasonStoppedByUser)
	ReasonRuntimeRestarted       ReasonCode = ReasonCode(goalworkflow.GoalReasonRuntimeRestarted)
	ReasonRunStartFailed         ReasonCode = ReasonCode(goalworkflow.GoalReasonRunStartFailed)
	ReasonAwaitingInput          ReasonCode = ReasonCode(goalworkflow.GoalReasonAwaitingInput)
	ReasonTerminalOutcomeMissing ReasonCode = ReasonCode(goalworkflow.GoalReasonTerminalOutcomeMissing)
	ReasonRunNotCompleted        ReasonCode = ReasonCode(goalworkflow.GoalReasonRunNotCompleted)

	ReasonBlockedByModel ReasonCode = ReasonCode(goalworkflow.GoalReasonBlockedByModel)
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

func (r Reason) validateFor(status Status) error {
	switch status {
	case StatusActive, StatusComplete:
		if r != (Reason{}) {
			return fmt.Errorf("%w: %s Goal must not carry a stop reason", ErrInvalid, status)
		}
	case StatusPaused, StatusBlocked:
		validated, err := newReason(status, r.code, r.runOutcome, r.explanation)
		if err != nil || validated != r {
			return fmt.Errorf("%w: invalid %s reason: %v", ErrInvalid, status, err)
		}
	}
	return nil
}
