package runs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

const (
	terminalCommitRetryBase    = 100 * time.Millisecond
	terminalCommitRetryMaximum = 5 * time.Second
)

// The same terminal write must survive temporary failure when only it can
// preserve the original settled outcome, external uncertainty, or the exact
// cancellation acknowledgement.
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

// boundaryDurability says what a failed Run boundary write may do. A write that
// alone preserves a settled outcome, external uncertainty or a cancellation
// acknowledgement must be retried until it commits; any other may abort.
type boundaryDurability uint8

const (
	abortOnFailure boundaryDurability = iota
	retryUntilCommitted
)

// durability keeps a commit carrying a cancellation acknowledgement that no
// other write can deliver.
func (e EventCommit) durability() boundaryDurability {
	if e.childCancellation != nil || e.rootCancellation != nil {
		return retryUntilCommitted
	}
	return abortOnFailure
}

// durability keeps a tree write that settles a Run or delivers an
// acknowledgement.
func (c ExecutionTreeCommit) durability() boundaryDurability {
	for _, event := range c.Events {
		if event.State == StateTerminalize || event.durability() == retryUntilCommitted {
			return retryUntilCommitted
		}
	}
	return abortOnFailure
}

// endingDurability keeps a terminal write recording unresolved external effects
// or a lost outcome, facts recovery cannot reconstruct.
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
			slog.ErrorContext(ctx, "runs: boundary commit failed, retrying until it commits", "session.id", sessionID, "run.id", runID, "error", err)
			reported = true
		}
		if waitErr := retry.wait(ctx); waitErr != nil {
			return errors.Join(err, waitErr)
		}
	}
}
