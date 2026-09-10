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
// The zero value is unconfigured: a schedule with no floor would busy-spin.
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
		if delay > b.maximum/2 {
			return b.maximum, nil
		}
		delay *= 2
	}
	return min(delay, b.maximum), nil
}

func (b Backoff) Wait(ctx context.Context, failure int) error {
	delay, err := b.Delay(failure)
	if err != nil {
		return err
	}
	return Wait(ctx, delay)
}

// Wait pauses before the next attempt. Cancellation is decided before the
// delay is ever consulted: a timer that is already ready would otherwise race
// a closed Done channel, and a select between two ready cases lets a canceled
// loop take another attempt.
func Wait(ctx context.Context, delay time.Duration) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
