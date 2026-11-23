package runs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

const (
	terminalCommitRetryBase    = 100 * time.Millisecond
	terminalCommitRetryMaximum = 5 * time.Second
)

type terminalCommitRetry struct {
	next time.Duration
}

func (r *terminalCommitRetry) wait(ctx context.Context) error {
	delay := r.advance()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (r *terminalCommitRetry) advance() time.Duration {
	if r.next <= 0 {
		r.next = terminalCommitRetryBase
		return r.next
	}
	if r.next >= terminalCommitRetryMaximum/2 {
		r.next = terminalCommitRetryMaximum
		return r.next
	}
	r.next *= 2
	return r.next
}

type boundaryDurability uint8

const (
	abortOnFailure boundaryDurability = iota
	retryUntilCommitted
)

func (e EventCommit) durability() boundaryDurability {
	if e.childCancellation != nil || e.rootCancellation != nil {
		return retryUntilCommitted
	}
	return abortOnFailure
}

func (c ExecutionTreeCommit) durability() boundaryDurability {
	for _, event := range c.Events {
		if event.State == StateTerminalize || event.durability() == retryUntilCommitted {
			return retryUntilCommitted
		}
	}
	return abortOnFailure
}

func endingDurability(fact ExecutionFact) boundaryDurability {
	end, ended := fact.(SegmentEnded)
	if ended && (len(end.unresolvedEffects) > 0 || end.Ending.Outcome() == run.OutcomeLost) {
		return retryUntilCommitted
	}
	return abortOnFailure
}

func commitRunBoundary(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, write func() error, durability boundaryDurability) error {
	retry := terminalCommitRetry{}
	reported := false
	for {
		err := write()
		if err == nil || durability == abortOnFailure {
			return err
		}
		if !reported {
			slog.ErrorContext(ctx, "runs: boundary commit failed, retrying until it commits", telemetry.SessionID(sessionID), telemetry.RunID(runID), telemetry.Error(err))
			reported = true
		}
		if waitErr := retry.wait(ctx); waitErr != nil {
			return errors.Join(err, waitErr)
		}
	}
}
