package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// ErrInvalidExecutorRef reports an incomplete or cross-session executor
// identity.
var ErrInvalidExecutorRef = errors.New("execution: invalid executor reference")

// ExecutorRef is the implementation-neutral durable address of the execution
// backing a Run. A resumed Run keeps this identity while opening a new Segment.
type ExecutorRef struct {
	SessionID  string
	ExecutorID string
}

// ValidateFor checks that the executor returned a complete identity bound to
// the admitted session.
func (e ExecutorRef) ValidateFor(sessionID string) error {
	if _, err := resourceid.ParseSession(e.SessionID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExecutorRef, err)
	}
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return fmt.Errorf("%w: admitted %v", ErrInvalidExecutorRef, err)
	}
	if _, err := runtimeidentity.ParseExecutor(e.ExecutorID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExecutorRef, err)
	}
	if e.SessionID != sessionID {
		return fmt.Errorf("%w: executor session %q does not match admitted session %q", ErrInvalidExecutorRef, e.SessionID, sessionID)
	}
	return nil
}
