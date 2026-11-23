package retry

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

const reconnectBaseDelay = 50 * time.Millisecond

func ReconnectDelay(n int, failure error) (time.Duration, bool, error) {
	if !IsReconnectable(failure) {
		return 0, false, nil
	}
	delay, err := (Backoff{base: reconnectBaseDelay, maximum: time.Second}).Delay(n)
	if err != nil {
		return 0, false, err
	}
	if errors.Is(failure, conversation.ErrCommandInProgress) {
		delay = time.Second
	}
	return delay, true, nil
}

func IsReconnectable(err error) bool {
	return errors.Is(err, conversation.ErrDisconnected) || errors.Is(err, conversation.ErrCommandInProgress)
}
