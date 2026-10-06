package runs

import "errors"

// ErrInvalidExecutorRef reports an incomplete or cross-session executor
// identity.
var ErrInvalidExecutorRef = errors.New("execution: invalid executor reference")

// ExecutorRef is the implementation-neutral durable address of the execution
// backing a Run. A resumed Run keeps this identity while opening a new Segment.
type ExecutorRef struct {
	SessionID  string
	ExecutorID string
}
