package retry

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// ReconnectDelay retries classified transport failures for as long as the
// observer remains alive. Only the delay is bounded, never the attempt count.
func ReconnectDelay(n int, failure error) (time.Duration, bool, error) {
	if !IsReconnectable(failure) {
		return 0, false, nil
	}
	delay, err := (Backoff{base: 50 * time.Millisecond, maximum: time.Second}).Delay(n)
	if err != nil {
		return 0, false, err
	}
	if errors.Is(failure, agent.ErrCommandInProgress) {
		delay = time.Second
	}
	return delay, true, nil
}

// IsReconnectable reports whether another transport attempt can repair the
// classified failure. Business, validation, and compatibility errors are permanent.
func IsReconnectable(err error) bool {
	return errors.Is(err, agent.ErrDisconnected) || errors.Is(err, agent.ErrCommandInProgress)
}
