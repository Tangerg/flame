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

// terminalCommitRetry owns the retry cadence for the one terminal write
// that may not be abandoned: an external Effect whose durable outcome is
// unknown. The Run remains blocked until commit or owner cancellation, while a
// persistent store outage cannot cause a fixed-rate write storm.
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

func (s *segmentPump) publishTerminal(route *executorRoute, batch reductionBatch) (reductionPublication, error) {
	retry := terminalCommitRetry{}
	reported := false
	for {
		publication, err := s.publisher.publishTerminalAtomically(s.ownerCtx, route, batch)
		if err == nil {
			return publication, nil
		}
		if !reported {
			slog.ErrorContext(s.ownerCtx, "runs: terminal commit failed, retrying until it commits", "session.id", s.spec.SessionID, "run.id", route.runID, "error", err)
			reported = true
		}
		if waitErr := retry.wait(s.ownerCtx); waitErr != nil {
			return reductionPublication{}, errors.Join(err, waitErr)
		}
	}
}
