package runs

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

var ErrInvalidExecutorRef = errors.New("execution: invalid executor reference")

type ExecutorRef struct {
	SessionID  identity.SessionID
	ExecutorID string
}
