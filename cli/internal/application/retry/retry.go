// Package retry owns retry schedules and classified transport-recovery policy.
package retry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidBackoff = errors.New("retry backoff is invalid")

// Backoff is an unbounded retry schedule whose delay has a finite ceiling.
// The caller's context, rather than an attempt budget, decides its lifetime.
type Backoff struct {
	base    time.Duration
	maximum time.Duration
}

func NewBackoff(base, maximum time.Duration) (Backoff, error) {
	backoff := Backoff{base: base, maximum: maximum}
	if err := backoff.Validate(); err != nil {
		return Backoff{}, err
	}
	return backoff, nil
}

func (b Backoff) Validate() error {
	if b.base <= 0 || b.maximum < b.base {
		return fmt.Errorf("%w: require 0 < base <= maximum", ErrInvalidBackoff)
	}
	return nil
}

func (b Backoff) Delay(failure int) (time.Duration, error) {
	if failure <= 0 {
		return 0, fmt.Errorf("%w: failure number must be positive", ErrInvalidBackoff)
	}
	if err := b.Validate(); err != nil {
		return 0, err
	}
	delay := b.base
	for range failure - 1 {
		if delay >= b.maximum-delay {
			return b.maximum, nil
		}
		delay *= 2
	}
	return delay, nil
}

func (b Backoff) Wait(ctx context.Context, failure int) error {
	delay, err := b.Delay(failure)
	if err != nil {
		return err
	}
	return Wait(ctx, delay)
}

func Wait(ctx context.Context, delay time.Duration) error {
	if cause := context.Cause(ctx); cause != nil || delay <= 0 {
		return cause
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	return context.Cause(ctx)
}
