package runs

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// executorRequest is the package-private ownership protocol for an executor
// control request whose durable decision belongs to the Run pump. Cancellation
// may withdraw a queued request; after claim, the producer must observe the
// consumer's conclusive result so it cannot act on an ambiguous write-set.
// The decision is immutable after done closes and remains available to its producer.
type executorRequest[T any] struct {
	mu     sync.Mutex
	state  executorRequestState
	result executorRequestResult[T]
	done   chan struct{}
}

type executorRequestState uint8

const (
	executorRequestPending executorRequestState = iota
	executorRequestClaimed
	executorRequestCompleted
	executorRequestCanceled
)

type executorRequestResult[T any] struct {
	value T
	err   error
}

func newExecutorRequest[T any]() *executorRequest[T] {
	return &executorRequest[T]{done: make(chan struct{})}
}

func (e *executorRequest[T]) claim() bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != executorRequestPending {
		return false
	}
	e.state = executorRequestClaimed
	return true
}

func (e *executorRequest[T]) complete(value T, err error) error {
	if e == nil {
		return errors.New("runs: complete nil executor request")
	}
	e.mu.Lock()
	if e.state != executorRequestClaimed {
		state := e.state
		e.mu.Unlock()
		return fmt.Errorf("runs: complete executor request in %s state; want claimed", state)
	}
	e.state = executorRequestCompleted
	e.result = executorRequestResult[T]{value: value, err: err}
	close(e.done)
	e.mu.Unlock()
	return nil
}

func (e *executorRequest[T]) await(ctx context.Context) (T, error) {
	var zero T
	if e == nil || e.done == nil {
		return zero, errors.New("runs: await malformed executor request")
	}
	if ctx == nil {
		return zero, errors.New("runs: executor request context is required")
	}
	select {
	case <-e.done:
		return e.result.value, e.result.err
	case <-ctx.Done():
	}

	e.mu.Lock()
	switch e.state {
	case executorRequestPending:
		e.state = executorRequestCanceled
		e.result.err = context.Cause(ctx)
		close(e.done)
	case executorRequestClaimed, executorRequestCompleted, executorRequestCanceled:
	default:
		e.mu.Unlock()
		return zero, errors.New("runs: executor request has an invalid state")
	}
	e.mu.Unlock()
	<-e.done
	return e.result.value, e.result.err
}

func (e executorRequestState) String() string {
	switch e {
	case executorRequestPending:
		return "pending"
	case executorRequestClaimed:
		return "claimed"
	case executorRequestCompleted:
		return "completed"
	case executorRequestCanceled:
		return "canceled"
	default:
		return "invalid"
	}
}
