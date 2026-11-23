package exec

import (
	"errors"
	"fmt"
	"time"
)

var errInvalidTimeout = errors.New("exec: invalid timeout")

type Timeout struct {
	duration time.Duration
	enabled  bool
}

func NewTimeout(duration time.Duration) (Timeout, error) {
	if duration <= 0 {
		return Timeout{}, fmt.Errorf("%w: duration must be positive", errInvalidTimeout)
	}
	return Timeout{duration: duration, enabled: true}, nil
}

func (t Timeout) Validate() error {
	if t.enabled && t.duration <= 0 {
		return fmt.Errorf("%w: enabled duration must be positive", errInvalidTimeout)
	}
	if !t.enabled && t.duration != 0 {
		return fmt.Errorf("%w: disabled timeout carries a duration", errInvalidTimeout)
	}
	return nil
}

func (t Timeout) Duration() (time.Duration, bool) { return t.duration, t.enabled }
