package agentexec

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

// errInteractionReleased reports that a projection could not be delivered
// because this session's owner began releasing it. Release is a lifecycle fact
// rather than a Run failure: the owner that released the session publishes the
// authoritative terminal, so a reconciler that meets this has no fact of its own.
var errInteractionReleased = errors.New("agentexec: execution released before projection")

// releasedDuringProjection reports that a reconciler lost its ability to publish
// rather than that it found a broken Run. A caller that stopped waiting and an
// owner that began release are both this session ending, not failing.
func releasedDuringProjection(err error) bool {
	return errors.Is(err, errInteractionReleased) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// interactionLifetime owns every goroutine and channel whose lifetime is the
// staged Interaction session. The execution context follows the external
// owner; its reconciliation child can stop before post-Run maintenance without
// canceling that maintenance. Release and finish remain one-shot transitions
// joined through the worker groups.
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

func (i *interactionLifetime) beginRelease() {
	i.releaseOnce.Do(func() {
		i.stopRelease()
		i.stopExecution()
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
	case <-i.releasing.Done():
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
	case <-i.releasing.Done():
		return errInteractionReleased
	case <-ctx.Done():
		return ctx.Err()
	}
}

// publicationContext preserves observed results after execution cancellation.
// Only release ends publication, when the product owner stops consuming facts.
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
