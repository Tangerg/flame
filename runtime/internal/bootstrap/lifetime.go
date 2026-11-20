package bootstrap

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/frame"
)

type runtimeLifetime struct {
	delivery     *delivery.Endpoint
	stopRuntime  context.CancelFunc
	recoveryDone <-chan struct{}

	context      context.Context
	closeMu      sync.Mutex
	phase        lifetimePhase
	shutdownWait shutdownWaitPolicy
	shutdown     *shutdownAttempt

	goalDriver      shutdownComponent
	mcpCoordinator  shutdownComponent
	runSegments     shutdownComponent
	executor        shutdownComponent
	runEffectTasks  taskOwner
	toolResources   []*teardownStep
	hostResources   []*teardownStep
	resourceGraph   *shutdownSequence
	frameworkCauses *frame.CloseCauses
}

// lifetimePhase orders Runtime shutdown. Stopping is entered once, by the first
// attempt; a failed attempt leaves it there so a retry joins the remaining
// teardown instead of broadcasting cancellation again.
type lifetimePhase uint8

const (
	lifetimeRunning lifetimePhase = iota
	lifetimeStopping
	lifetimeClosed
)

func newRuntimeLifetime(ctx context.Context, resources []terminalResource) *runtimeLifetime {
	return &runtimeLifetime{
		context:       ctx,
		shutdownWait:  defaultShutdownWaitPolicy(),
		hostResources: terminalResources(resources),
	}
}

type shutdownAttempt struct {
	done chan struct{}
	err  error
}

func (a *shutdownAttempt) completed() bool {
	select {
	case <-a.done:
		return true
	default:
		return false
	}
}

type shutdownComponent interface {
	BeginShutdown()
	AwaitShutdown(ctx context.Context) error
}

type taskOwner interface {
	Drain(ctx context.Context) error
	Cancel()
	Wait(ctx context.Context) error
}

const runEffectDrainTimeout = 5 * time.Second

// Close shuts the complete Runtime down in reverse dependency order.
// The first caller starts one shutdown attempt that stops
// producers, drains then cancels accepted maintenance, joins components, and
// finally enters terminal resource teardown.
// Each caller has a bounded wait, but its deadline cannot abandon or duplicate
// that generation. A completed component error permits a later Close to start
// one new generation. Terminal diagnostics remain the result of every later
// Close without replaying resource teardown.
func (i *Instance) Close() error {
	return closeRuntimeLifetime(i.lifetime)
}

func closeRuntimeLifetime(lifetime *runtimeLifetime) (err error) {
	defer func() {
		if lifetime.frameworkCauses != nil {
			if cause := lifetime.frameworkCauses.Err(); cause != nil && !errors.Is(err, cause) {
				err = errors.Join(err, cause)
			}
		}
	}()
	// Preserve instance trace values, but never let the caller that happened to
	// start Close cancel the owner generation.
	ownerCtx := context.WithoutCancel(lifetime.context)
	timeout, err := shutdownWaitTimeout(lifetime.shutdownWait)
	if err != nil {
		return err
	}
	attempt, closed := beginShutdown(ownerCtx, lifetime)
	if closed {
		return attempt.err
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := completion.Wait(waitCtx, attempt.done); err != nil {
		return err
	}
	return attempt.err
}

func beginShutdown(
	ownerCtx context.Context,
	lifetime *runtimeLifetime,
) (attempt *shutdownAttempt, closed bool) {
	lifetime.closeMu.Lock()
	defer lifetime.closeMu.Unlock()
	if lifetime.phase == lifetimeClosed {
		return lifetime.shutdown, true
	}
	attempt = lifetime.shutdown
	if attempt == nil || attempt.completed() {
		attempt = &shutdownAttempt{done: make(chan struct{})}
		lifetime.shutdown = attempt
		go runShutdown(ownerCtx, lifetime, attempt)
	}
	return attempt, false
}

func runShutdown(
	ownerCtx context.Context,
	lifetime *runtimeLifetime,
	attempt *shutdownAttempt,
) {
	components := []shutdownComponent{
		lifetime.goalDriver,
		lifetime.mcpCoordinator,
		lifetime.runSegments,
	}
	if lifetime.beginStopping() {
		lifetime.broadcastShutdown(components)
	}
	if err := lifetime.joinWork(ownerCtx, components); err != nil {
		finishShutdown(lifetime, attempt, false, err)
		return
	}
	settled, resourceErr := lifetime.teardownGraph().shutdown(ownerCtx)
	finishShutdown(lifetime, attempt, settled, resourceErr)
}

// beginStopping reports whether this call moved a running lifetime to
// stopping; only that call broadcasts shutdown.
func (l *runtimeLifetime) beginStopping() bool {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.phase != lifetimeRunning {
		return false
	}
	l.phase = lifetimeStopping
	return true
}

// broadcastShutdown stops new work everywhere before anything is joined.
// Delivery is acquired after assembly, so a startup that fails before then
// rolls back a graph that never had an endpoint to stop.
func (l *runtimeLifetime) broadcastShutdown(components []shutdownComponent) {
	if l.delivery != nil {
		l.delivery.BeginShutdown()
	}
	if l.stopRuntime != nil {
		l.stopRuntime()
	}
	// An accepted invocation can await component-owned post-commit work.
	// Broadcast component cancellation before joining delivery to avoid
	// making that work wait for the shutdown that is waiting for it.
	for _, component := range components {
		if component != nil {
			component.BeginShutdown()
		}
	}
}

// joinWork waits for recovery, components, run effects, delivery and the
// executor, in the order each depends on the previous having stopped.
func (l *runtimeLifetime) joinWork(ownerCtx context.Context, components []shutdownComponent) error {
	if err := completion.Wait(ownerCtx, l.recoveryDone); err != nil {
		return err
	}
	var errs []error
	for _, component := range components {
		if component != nil {
			errs = append(errs, component.AwaitShutdown(ownerCtx))
		}
	}
	if l.runEffectTasks != nil {
		errs = append(errs, l.drainRunEffects(ownerCtx))
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	// Run tasks can finish a keyed cancellation after its invocation returns.
	// Join those producers before flushing delivery's known outcomes.
	if l.delivery != nil {
		if err := l.delivery.AwaitShutdown(ownerCtx); err != nil {
			return err
		}
	}
	if l.executor != nil {
		l.executor.BeginShutdown()
		return l.executor.AwaitShutdown(ownerCtx)
	}
	return nil
}

// drainRunEffects lets accepted best-effort work (notably Session title
// generation) settle for a bounded window; run producers are already joined,
// so nothing new is admitted. A stuck provider is then canceled through the
// normal owner.
func (l *runtimeLifetime) drainRunEffects(ownerCtx context.Context) error {
	drainCtx, cancelDrain := context.WithTimeout(ownerCtx, runEffectDrainTimeout)
	_ = l.runEffectTasks.Drain(drainCtx)
	cancelDrain()
	l.runEffectTasks.Cancel()
	return l.runEffectTasks.Wait(ownerCtx)
}

// teardownGraph is the one reverse-dependency teardown of every resource.
// Host resources are acquired before tool resources, so concatenating them in
// creation order lets the Sequence own the whole graph in one
// self-continuing generation.
func (l *runtimeLifetime) teardownGraph() *shutdownSequence {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.resourceGraph == nil {
		steps := make([]*teardownStep, 0, len(l.hostResources)+len(l.toolResources))
		steps = append(steps, l.hostResources...)
		steps = append(steps, l.toolResources...)
		l.resourceGraph = newShutdownSequence(steps)
	}
	return l.resourceGraph
}

func finishShutdown(
	lifetime *runtimeLifetime,
	attempt *shutdownAttempt,
	closed bool,
	err error,
) {
	lifetime.closeMu.Lock()
	defer lifetime.closeMu.Unlock()
	attempt.err = err
	if closed {
		lifetime.toolResources = nil
		lifetime.hostResources = nil
		lifetime.phase = lifetimeClosed
	}
	close(attempt.done)
}
