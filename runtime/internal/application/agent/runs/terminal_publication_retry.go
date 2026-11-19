package runs

import (
	"context"
	"errors"
	"log/slog"
	"time"
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

func commitRunBoundary(ctx context.Context, sessionID, runID string, write func() error, retryOnFailure bool) error {
	retry := terminalCommitRetry{}
	reported := false
	for {
		err := write()
		if err == nil || !retryOnFailure {
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
