// Package retry owns transport-agnostic waiting and exponential backoff.
package retry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidBackoff = errors.New("retry backoff is invalid")

type backoffMode uint8

const (
	backoffInvalid backoffMode = iota
	backoffImmediate
	backoffBounded
)

// Backoff is an unbounded retry schedule whose delay has a finite ceiling.
// The caller's context, rather than an attempt budget, decides its lifetime.
type Backoff struct {
	mode    backoffMode
	base    time.Duration
	maximum time.Duration
}

func ImmediateBackoff() Backoff { return Backoff{mode: backoffImmediate} }

func NewBackoff(base, maximum time.Duration) (Backoff, error) {
	if base <= 0 || maximum < base {
		return Backoff{}, fmt.Errorf("%w: require 0 < base <= maximum", ErrInvalidBackoff)
	}
	return Backoff{mode: backoffBounded, base: base, maximum: maximum}, nil
}

func (b Backoff) Validate() error {
	switch b.mode {
	case backoffImmediate:
		if b.base != 0 || b.maximum != 0 {
			return fmt.Errorf("%w: immediate policy carries durations", ErrInvalidBackoff)
		}
		return nil
	case backoffBounded:
		if b.base <= 0 || b.maximum < b.base {
			return fmt.Errorf("%w: bounded policy has invalid durations", ErrInvalidBackoff)
		}
		return nil
	default:
		return fmt.Errorf("%w: policy is not configured", ErrInvalidBackoff)
	}
}

func (b Backoff) Delay(failure int) (time.Duration, error) {
	if failure <= 0 {
		return 0, fmt.Errorf("%w: failure number must be positive", ErrInvalidBackoff)
	}
	if err := b.Validate(); err != nil {
		return 0, err
	}
	switch b.mode {
	case backoffImmediate:
		return 0, nil
	case backoffBounded:
		delay := b.base
		for range failure - 1 {
			if delay >= b.maximum/2 {
				return b.maximum, nil
			}
			delay *= 2
		}
		return min(delay, b.maximum), nil
	}
	return 0, ErrInvalidBackoff
}

func (b Backoff) Wait(ctx context.Context, failure int) error {
	delay, err := b.Delay(failure)
	if err != nil {
		return err
	}
	return Wait(ctx, delay)
}

func Wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return context.Cause(ctx)
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
