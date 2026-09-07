// Package mutation owns acknowledgement semantics for idempotent commands.
package mutation

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/retry"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

// ErrReplayGuaranteeUnavailable fences a command whose stable identity can no
// longer be proven replayable by the runtime that originally owned it.
var ErrReplayGuaranteeUnavailable = errors.New("mutation replay guarantee is unavailable")

const (
	acknowledgementRetryBase    = 50 * time.Millisecond
	acknowledgementRetryMaximum = time.Second
)

// AcknowledgementBackoff returns the shared retry schedule for a CLI command
// whose durable outcome is uncertain. Returning a value prevents one caller
// from mutating the policy observed by another.
func AcknowledgementBackoff() retry.Backoff {
	backoff, err := retry.NewBackoff(acknowledgementRetryBase, acknowledgementRetryMaximum)
	if err != nil {
		panic("invalid static acknowledgement backoff: " + err.Error())
	}
	return backoff
}

// Outcome is the authoritative settlement state shared by every durable CLI
// mutation. The zero value is invalid so an uninitialized result cannot be
// mistaken for a deliberately preserved unknown outcome.
type Outcome string

const (
	Unknown   Outcome = "unknown"
	Rejected  Outcome = "rejected"
	Confirmed Outcome = "confirmed"
)

func (o Outcome) Valid() bool {
	switch o {
	case Unknown, Rejected, Confirmed:
		return true
	default:
		return false
	}
}

func (o Outcome) String() string { return string(o) }

// Admission runs immediately before each real mutation attempt. Durable
// callers use it to enforce the runtime replay guarantee at the actual I/O
// boundary rather than only when a recovery workflow begins.
type Admission func() error

// ReplayAdmission checks the Runtime store and the recorded deadline before
// every attempt. The policy clock advances while the connection remains fixed.
func ReplayAdmission(policy commandreplay.Policy, guard commandreplay.Guard) Admission {
	return func() error {
		if !policy.Replayable(guard) {
			return ErrReplayGuaranteeUnavailable
		}
		return nil
	}
}

// AcknowledgementUncertain reports whether a mutation may have committed even
// though its acknowledgement was not observed. Callers must retry the same
// command identity; a fresh identity could execute the user's intent twice.
func AcknowledgementUncertain(err error) bool {
	return errors.Is(err, agent.ErrDisconnected) ||
		errors.Is(err, agent.ErrCommandInProgress) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// OutcomeUnknown includes failures which cannot be retried against the current
// runtime but still do not prove that an earlier attempt was refused. A store
// mismatch is fenced before current-store admission; it says nothing about the
// same command's outcome in the store that originally owned it.
func OutcomeUnknown(err error) bool {
	return AcknowledgementUncertain(err) ||
		errors.Is(err, agent.ErrCommandStoreMismatch) ||
		errors.Is(err, ErrReplayGuaranteeUnavailable)
}

// ConfirmAdmitted retries one idempotent mutation until its acknowledgement is
// observed, a definitive refusal arrives, or the owning context ends. The
// attempt closure must capture and reuse the same mutation identity. Admission
// runs immediately before every attempt, including the first; nil admits every
// attempt.
// An admission failure is an unknown outcome: an earlier call may have
// committed even though its replay guarantee has since expired.
func ConfirmAdmitted[T any](
	ctx context.Context,
	backoff retry.Backoff,
	admit Admission,
	attempt func(context.Context) (T, error),
) (T, error) {
	if err := backoff.Validate(); err != nil {
		var zero T
		return zero, err
	}
	for failures := 0; ; {
		if cause := context.Cause(ctx); cause != nil {
			var zero T
			return zero, cause
		}
		if admit != nil {
			if err := admit(); err != nil {
				var zero T
				return zero, err
			}
		}
		result, err := attempt(ctx)
		if err == nil || !AcknowledgementUncertain(err) {
			return result, err
		}
		failures++
		if err := backoff.Wait(ctx, failures); err != nil {
			var zero T
			return zero, err
		}
	}
}
