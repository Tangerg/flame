package runs

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

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

func (e *executorRequest[T]) claim() (claimedRequest[T], bool) {
	if e == nil {
		return claimedRequest[T]{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != executorRequestPending {
		return claimedRequest[T]{}, false
	}
	e.state = executorRequestClaimed
	return claimedRequest[T]{request: e}, true
}

type claimedRequest[T any] struct{ request *executorRequest[T] }

func (c claimedRequest[T]) settle(value T, err error) {
	e := c.request
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != executorRequestClaimed {
		panic(fmt.Sprintf("runs: settle executor request in %s state", e.state))
	}
	e.state = executorRequestCompleted
	e.result = executorRequestResult[T]{value: value, err: err}
	close(e.done)
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
