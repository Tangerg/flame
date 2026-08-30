// Package reconnect owns transport retry policy shared by interactive and
// headless delivery adapters. It classifies symbolic agent-port errors, never error
// strings, and contains no runtime or terminal implementation.
package reconnect

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/cli/internal/agent"
)

var ErrInvalidPolicy = errors.New("reconnect policy is invalid")

const (
	defaultRetryBase             = 50 * time.Millisecond
	defaultRetryMaximum          = time.Second
	commandInProgressMinimumWait = time.Second
)

type Policy struct {
	configured bool
	attempts   int
	base       time.Duration
	maximum    time.Duration
}

func New(attempts int) (Policy, error) {
	return newPolicy(attempts, defaultRetryBase, defaultRetryMaximum)
}

func Disabled() Policy {
	return Policy{
		configured: true,
		base:       defaultRetryBase,
		maximum:    defaultRetryMaximum,
	}
}

func newPolicy(attempts int, base, maximum time.Duration) (Policy, error) {
	if attempts < 0 || base <= 0 || maximum < base {
		return Policy{}, fmt.Errorf("%w: require non-negative attempts and 0 < base <= maximum", ErrInvalidPolicy)
	}
	return Policy{configured: true, attempts: attempts, base: base, maximum: maximum}, nil
}

func (p Policy) AttemptLimit() int { return p.attempts }

func (p Policy) Validate() error {
	if !p.configured || p.attempts < 0 || p.base <= 0 || p.maximum < p.base {
		return ErrInvalidPolicy
	}
	return nil
}

// Next reports the delay before retrying failure number n, counted from one.
func (p Policy) Next(n int, err error) (time.Duration, bool, error) {
	if err := p.Validate(); err != nil {
		return 0, false, err
	}
	if n <= 0 {
		return 0, false, fmt.Errorf("%w: failure number must be positive", ErrInvalidPolicy)
	}
	if n > p.attempts || !Retryable(err) {
		return 0, false, nil
	}
	delay := p.base
	for range n - 1 {
		if delay >= p.maximum/2 {
			delay = p.maximum
			break
		}
		delay *= 2
	}
	if errors.Is(err, agent.ErrCommandInProgress) {
		delay = max(delay, commandInProgressMinimumWait)
	}
	return min(delay, p.maximum), true, nil
}

// Retryable reports whether retrying can repair the classified transport
// failure. Business, validation, and compatibility errors are permanent.
func Retryable(err error) bool {
	return errors.Is(err, agent.ErrDisconnected) || errors.Is(err, agent.ErrCommandInProgress)
}
