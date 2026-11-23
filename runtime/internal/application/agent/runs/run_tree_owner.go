package runs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

const runCleanupTimeout = 5 * time.Second

type runTreeOwner struct {
	observation     sync.Mutex
	mu              sync.Mutex
	cancel          context.CancelFunc
	taskContext     context.Context
	hub             *journal
	done            chan struct{}
	completionErr   error
	terminalRuns    map[identity.RunID]terminalRun
	executorMembers map[identity.RunID]string
	childCancel     *childCancellation
	rootCancel      *rootCancellation
	activation      segmentActivation
	interrupt       interruptBoundary
}

type terminalRun struct {
	run          run.Run
	cancellation CancellationID
}

type activationPhase uint8

const (
	activationPending activationPhase = iota
	activationStarted
	activationResolved
	activationSuppressed
)

type segmentActivation struct {
	done  chan struct{}
	phase activationPhase
	err   error
}

func newRunTreeOwner(cancel context.CancelFunc, taskContext context.Context, hub *journal) *runTreeOwner {
	return &runTreeOwner{
		cancel:          cancel,
		taskContext:     taskContext,
		hub:             hub,
		done:            make(chan struct{}),
		activation:      segmentActivation{done: make(chan struct{})},
		executorMembers: make(map[identity.RunID]string),
	}
}

func (r *runTreeOwner) beginExecution(
	ctx context.Context,
	begin func(context.Context) error,
) (canceled bool, err error) {
	r.mu.Lock()
	if r.activation.phase != activationPending {
		r.mu.Unlock()
		return false, errors.New("runs: segment activation already resolved")
	}
	if r.rootCancel != nil {
		r.activation.phase = activationSuppressed
		close(r.activation.done)
		r.mu.Unlock()
		return true, nil
	}
	r.activation.phase = activationStarted
	r.mu.Unlock()

	if begin != nil {
		err = begin(ctx)
	}
	r.mu.Lock()
	r.activation.err = err
	r.activation.phase = activationResolved
	close(r.activation.done)
	r.mu.Unlock()
	return false, err
}

// A Run is durably Running before its executor activates, so steer waits here
// instead of misreading the activation window as a finished Run.
func (r *runTreeOwner) awaitActivation(ctx context.Context) error {
	r.mu.Lock()
	done := r.activation.done
	r.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	r.mu.Lock()
	activationErr := r.activation.err
	r.mu.Unlock()
	if activationErr != nil {
		return fmt.Errorf("%w: segment activation failed: %w", ErrRunFinished, activationErr)
	}
	return nil
}

func (r *runTreeOwner) rejectActivation(cause error) error {
	if cause == nil {
		return errors.New("runs: activation rejection requires a cause")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activation.phase != activationPending {
		return errors.New("runs: segment activation already resolved")
	}
	r.activation.err = cause
	r.activation.phase = activationResolved
	close(r.activation.done)
	return nil
}

func (r *runTreeOwner) committedRunTerminal(runID identity.RunID) (terminalRun, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	terminal, found := r.terminalRuns[runID]
	return terminal, found
}

func (r *runTreeOwner) stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	cancel()
}

func (r *runTreeOwner) wait(ctx context.Context) error {
	if err := completion.Wait(ctx, r.done); err != nil {
		return err
	}
	return r.completionErr
}

func runCleanupContext(base context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(base), runCleanupTimeout)
}

func (r *runTreeOwner) cleanupContext() (context.Context, context.CancelFunc) {
	r.mu.Lock()
	taskContext := r.taskContext
	r.mu.Unlock()
	return runCleanupContext(taskContext)
}
