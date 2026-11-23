package execution

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

const (
	auxiliaryOperationTimeout = 15 * time.Second
	executionCleanupTimeout   = 15 * time.Second
)

var errInteractionReleased = errors.New("execution: execution released before projection")

func releasedDuringProjection(err error) bool {
	return errors.Is(err, errInteractionReleased) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

type interactionLifetime struct {
	execution       context.Context
	stopExecution   context.CancelFunc
	reconciling     context.Context
	stopReconciling context.CancelFunc
	events          chan runs.ExecutorEvent
	done            chan struct{}
	releasing       context.Context
	stopRelease     context.CancelFunc
	unknownWake     chan struct{}
	stateWake       chan struct{}
	releaseOnce     sync.Once
	finishOnce      sync.Once
	publication     sync.RWMutex
	workers         sync.WaitGroup
	reconcilers     sync.WaitGroup
}

func newInteractionLifetime(parent context.Context) interactionLifetime {
	lifetime, stop := context.WithCancel(parent)
	reconciling, stopReconciling := context.WithCancel(lifetime)
	releasing, stopRelease := context.WithCancel(context.WithoutCancel(parent))
	return interactionLifetime{
		execution:       lifetime,
		stopExecution:   stop,
		reconciling:     reconciling,
		stopReconciling: stopReconciling,
		events:          make(chan runs.ExecutorEvent, interactionEventBuffer),
		done:            make(chan struct{}),
		releasing:       releasing,
		stopRelease:     stopRelease,
		unknownWake:     make(chan struct{}, 1),
		stateWake:       make(chan struct{}, 1),
	}
}

func (i *interactionLifetime) start(
	await func(),
	reconcileUnknownEffects func(),
	reconcileExecutionState func(),
	readers ...func(),
) {
	i.workers.Add(1 + len(readers))
	for _, reader := range readers {
		go func() {
			defer i.workers.Done()
			reader()
		}()
	}
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

func (i *interactionLifetime) beginRelease() {
	i.releaseOnce.Do(func() {
		i.stopRelease()
		i.stopExecution()
	})
}

func (i *interactionLifetime) offer(event runs.ExecutorEvent) bool {
	i.publication.RLock()
	defer i.publication.RUnlock()
	select {
	case <-i.done:
		return false
	default:
	}
	select {
	case i.events <- event:
		return true
	default:
		return false
	}
}

func (i *interactionLifetime) send(event runs.ExecutorEvent) bool {
	i.publication.RLock()
	defer i.publication.RUnlock()
	select {
	case <-i.done:
		return false
	default:
	}
	select {
	case i.events <- event:
		return true
	case <-i.releasing.Done():
		return false
	case <-i.done:
		return false
	}
}

func (i *interactionLifetime) sendAuthoritative(
	ctx context.Context,
	event runs.ExecutorEvent,
) error {
	i.publication.RLock()
	defer i.publication.RUnlock()
	select {
	case <-i.done:
		return errInteractionReleased
	default:
	}
	select {
	case i.events <- event:
		return nil
	case <-i.releasing.Done():
		return errInteractionReleased
	case <-i.done:
		return errInteractionReleased
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *interactionLifetime) closePublication() {
	i.publication.Lock()
	defer i.publication.Unlock()
	close(i.events)
}

func (i *interactionLifetime) publicationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(i.releasing, cancel)
	if i.releasing.Err() != nil {
		cancel()
	}
	return bound, func() {
		stop()
		cancel()
	}
}

func (i *interactionLifetime) bind(ctx context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(i.execution, cancel)
	if i.execution.Err() != nil {
		cancel()
	}
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
