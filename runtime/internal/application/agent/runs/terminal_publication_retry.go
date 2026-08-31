package runs

import (
	"context"
	"time"
)

const (
	unknownEffectCommitRetryBase    = 100 * time.Millisecond
	unknownEffectCommitRetryMaximum = 5 * time.Second
)

// unknownEffectCommitRetry owns the retry cadence for the one terminal write
// that may not be abandoned: an external Effect whose durable outcome is
// unknown. The Run remains blocked until commit or owner cancellation, while a
// persistent store outage cannot cause a fixed-rate write storm.
type unknownEffectCommitRetry struct {
	next time.Duration
}

func (r *unknownEffectCommitRetry) wait(ctx context.Context) error {
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

func (r *unknownEffectCommitRetry) advance() time.Duration {
	if r.next <= 0 {
		r.next = unknownEffectCommitRetryBase
		return r.next
	}
	if r.next >= unknownEffectCommitRetryMaximum/2 {
		r.next = unknownEffectCommitRetryMaximum
		return r.next
	}
	r.next *= 2
	return r.next
}
