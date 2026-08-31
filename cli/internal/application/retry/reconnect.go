package retry

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

var ErrInvalidReconnectPolicy = errors.New("reconnect policy is invalid")

const (
	defaultReconnectBase         = 50 * time.Millisecond
	defaultReconnectMaximum      = time.Second
	commandInProgressMinimumWait = time.Second
)

// ReconnectPolicy is a finite transport-recovery budget. Backoff owns the
// schedule; this type adds retry admission and an attempt limit.
type ReconnectPolicy struct {
	attempts int
	backoff  Backoff
}

func NewReconnectPolicy(attempts int) (ReconnectPolicy, error) {
	return newReconnectPolicy(attempts, defaultReconnectBase, defaultReconnectMaximum)
}

func DisabledReconnectPolicy() ReconnectPolicy {
	policy, err := newReconnectPolicy(0, defaultReconnectBase, defaultReconnectMaximum)
	if err != nil {
		panic("invalid static reconnect policy: " + err.Error())
	}
	return policy
}

func newReconnectPolicy(attempts int, base, maximum time.Duration) (ReconnectPolicy, error) {
	if attempts < 0 {
		return ReconnectPolicy{}, fmt.Errorf("%w: attempts must be non-negative", ErrInvalidReconnectPolicy)
	}
	backoff, err := NewBackoff(base, maximum)
	if err != nil {
		return ReconnectPolicy{}, fmt.Errorf("%w: %v", ErrInvalidReconnectPolicy, err)
	}
	return ReconnectPolicy{attempts: attempts, backoff: backoff}, nil
}

func (p ReconnectPolicy) AttemptLimit() int { return p.attempts }

func (p ReconnectPolicy) Validate() error {
	if p.attempts < 0 {
		return ErrInvalidReconnectPolicy
	}
	if err := p.backoff.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidReconnectPolicy, err)
	}
	return nil
}

// Next reports the delay before retrying failure number n, counted from one.
func (p ReconnectPolicy) Next(n int, failure error) (time.Duration, bool, error) {
	if err := p.Validate(); err != nil {
		return 0, false, err
	}
	if n <= 0 {
		return 0, false, fmt.Errorf("%w: failure number must be positive", ErrInvalidReconnectPolicy)
	}
	if n > p.attempts || !IsReconnectable(failure) {
		return 0, false, nil
	}
	delay, err := p.backoff.Delay(n)
	if err != nil {
		return 0, false, fmt.Errorf("%w: %v", ErrInvalidReconnectPolicy, err)
	}
	if errors.Is(failure, agent.ErrCommandInProgress) {
		delay = max(delay, min(commandInProgressMinimumWait, p.backoff.maximum))
	}
	return delay, true, nil
}

// IsReconnectable reports whether another transport attempt can repair the
// classified failure. Business, validation, and compatibility errors are permanent.
func IsReconnectable(err error) bool {
	return errors.Is(err, agent.ErrDisconnected) || errors.Is(err, agent.ErrCommandInProgress)
}
