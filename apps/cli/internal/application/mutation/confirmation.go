package mutation

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

var ErrReplayGuaranteeUnavailable = errors.New("mutation replay guarantee is unavailable")

const (
	acknowledgementRetryBase    = 50 * time.Millisecond
	acknowledgementRetryMaximum = time.Second
)

func AcknowledgementBackoff() retry.Backoff {
	backoff, err := retry.NewBackoff(acknowledgementRetryBase, acknowledgementRetryMaximum)
	if err != nil {
		panic("invalid static acknowledgement backoff: " + err.Error())
	}
	return backoff
}

// The zero value is invalid so an uninitialized result cannot be mistaken for a
// deliberately preserved unknown outcome.
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

type Admission func() error

func ReplayAdmission(policy ReplayPolicy, guard authoring.ReplayGuard) Admission {
	return func() error {
		if !policy.Replayable(guard) {
			return ErrReplayGuaranteeUnavailable
		}
		return nil
	}
}

func AcknowledgementUncertain(err error) bool {
	return errors.Is(err, conversation.ErrDisconnected) ||
		errors.Is(err, conversation.ErrCommandInProgress) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func OutcomeUnknown(err error) bool {
	return AcknowledgementUncertain(err) ||
		errors.Is(err, conversation.ErrCommandConflict) ||
		errors.Is(err, authoring.ErrCommandNotDispatched) ||
		errors.Is(err, authoring.ErrCommandInputUnavailable) ||
		errors.Is(err, authoring.ErrCommandOutcomeUnknown) ||
		errors.Is(err, conversation.ErrCommandStoreMismatch) ||
		errors.Is(err, ErrReplayGuaranteeUnavailable)
}

// An admission failure is an unknown outcome: an earlier attempt may have
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
