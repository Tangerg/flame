package agentexec

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

// interactionLifetime owns every goroutine and channel whose lifetime is the
// staged Interaction session. The execution context follows the external
// owner; its reconciliation child can stop before post-Run maintenance without
// canceling that maintenance. Release and finish remain one-shot transitions
// joined through the worker groups.
type interactionLifetime struct {
	owner           context.Context
	execution       context.Context
	stopExecution   context.CancelFunc
	reconciling     context.Context
	stopReconciling context.CancelFunc
	events          chan runs.ExecutorEvent
	done            chan struct{}
	releasing       chan struct{}
	unknownWake     chan struct{}
	stateWake       chan struct{}
	releaseOnce     sync.Once
	finishOnce      sync.Once
	workers         sync.WaitGroup
	reconcilers     sync.WaitGroup
}

func newInteractionLifetime(parent context.Context) interactionLifetime {
	lifetime, stop := context.WithCancel(parent)
	reconciling, stopReconciling := context.WithCancel(lifetime)
	return interactionLifetime{
		owner:           parent,
		execution:       lifetime,
		stopExecution:   stop,
		reconciling:     reconciling,
		stopReconciling: stopReconciling,
		events:          make(chan runs.ExecutorEvent, interactionEventBuffer),
		done:            make(chan struct{}),
		releasing:       make(chan struct{}),
		unknownWake:     make(chan struct{}, 1),
		stateWake:       make(chan struct{}, 1),
	}
}

// start registers the complete session worker set before any worker can run.
// A Process may already be terminal when observation begins, so its awaiter is
// allowed to stop and join reconciliation immediately without racing a later
// WaitGroup registration.
func (i *interactionLifetime) start(
	await func(),
	reconcileUnknownEffects func(),
	reconcileExecutionState func(),
) {
	i.workers.Add(1)
	i.reconcilers.Add(2)
	go func() {
		defer i.reconcilers.Done()
		reconcileUnknownEffects()
	}()
	go func() {
		defer i.reconcilers.Done()
		reconcileExecutionState()
	}()
	go func() {
		defer i.workers.Done()
		await()
	}()
}

func (i *interactionLifetime) ownerCause() error { return context.Cause(i.owner) }

func (i *interactionLifetime) beginRelease() {
	i.releaseOnce.Do(func() {
		i.stopExecution()
		close(i.releasing)
	})
}

func (i *interactionLifetime) offer(event runs.ExecutorEvent) bool {
	select {
	case i.events <- event:
		return true
	default:
		return false
	}
}

func (i *interactionLifetime) send(event runs.ExecutorEvent) bool {
	select {
	case i.events <- event:
		return true
	case <-i.releasing:
		return false
	}
}

func (i *interactionLifetime) sendAuthoritative(
	ctx context.Context,
	event runs.ExecutorEvent,
) error {
	select {
	case i.events <- event:
		return nil
	case <-i.releasing:
		return errors.New("agentexec: execution released before authoritative fact commit")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *interactionLifetime) bind(ctx context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(i.execution, cancel)
	return bound, func() {
		stop()
		cancel()
	}
}

func (i *interactionLifetime) wakeUnknown() {
	select {
	case i.unknownWake <- struct{}{}:
	default:
	}
}

func (i *interactionLifetime) wakeState() {
	select {
	case i.stateWake <- struct{}{}:
	default:
	}
}
