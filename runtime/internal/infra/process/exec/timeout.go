package exec

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidTimeout = errors.New("exec: invalid timeout")

// Timeout is the optional hard lifetime of one shell command or wait. Its
// useful zero value is explicitly disabled; a present timeout is constructed
// from a strictly positive duration so numeric zero never doubles as policy.
type Timeout struct {
	duration time.Duration
	enabled  bool
}

// NewTimeout validates and freezes an enabled hard timeout.
func NewTimeout(duration time.Duration) (Timeout, error) {
	if duration <= 0 {
		return Timeout{}, fmt.Errorf("%w: duration must be positive", ErrInvalidTimeout)
	}
	return Timeout{duration: duration, enabled: true}, nil
}

// Validate checks the value's construction invariant.
func (t Timeout) Validate() error {
	if t.enabled && t.duration <= 0 {
		return fmt.Errorf("%w: enabled duration must be positive", ErrInvalidTimeout)
	}
	if !t.enabled && t.duration != 0 {
		return fmt.Errorf("%w: disabled timeout carries a duration", ErrInvalidTimeout)
	}
	return nil
}

// Duration returns the hard lifetime and whether it is enabled.
func (t Timeout) Duration() (time.Duration, bool) { return t.duration, t.enabled }
