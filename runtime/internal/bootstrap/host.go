package bootstrap

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/infra/teardown"
)

// Host owns the assembled application tier and its process-level close order
// (§13.2). Its application capsule exposes behavior only inside Bootstrap;
// process resources remain in the immutable shared shutdown graph.
type Host struct {
	application *hostApplication
	// lifetime owns the immutable shutdown graph shared by every Host copy.
	lifetime *hostLifetime
}

type hostLifetime struct {
	context      context.Context
	closeMu      sync.Mutex
	stopping     bool
	closed       bool
	shutdownWait shutdownWaitPolicy
	shutdown     *hostShutdownAttempt

	goalDriver     shutdownComponent
	mcpCoordinator shutdownComponent
	runCoordinator shutdownComponent
	executor       shutdownComponent
	runEffectTasks taskOwner
	toolResources  []*teardown.Step
	hostResources  []*teardown.Step
	resourceGraph  *teardown.Sequence
}

type hostShutdownAttempt struct {
	done      chan struct{}
	err       error
	completed bool
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

// Close shuts the assembled application tier down in reverse dependency order
// (§10.3). The first caller starts one Host-owned generation that stops
// producers, drains then cancels accepted maintenance, joins components, and
// finally enters terminal resource teardown.
// Each caller has a bounded wait, but its deadline cannot abandon or duplicate
// that generation. A completed component error permits a later Close to start
// one new generation; terminal resource diagnostics close the graph. Idempotent
// across Host copies once the graph has fully closed.
func (h *Host) Close() error {
	if h == nil || h.lifetime == nil {
		return nil
	}
	return closeHostLifetime(h.lifetime)
}

func closeHostLifetime(lifetime *hostLifetime) error {
	if lifetime == nil {
		return nil
	}
	// Preserve instance trace values, but never let the caller that happened to
	// start Close cancel the owner generation. A nil lifetime context occurs only
	// in direct Host tests and uses the same process-owner root as the wait.
	ownerCtx := context.Background()
	if lifetime.context != nil {
		ownerCtx = context.WithoutCancel(lifetime.context)
	}
	timeout, err := shutdownWaitTimeout(lifetime.shutdownWait)
	if err != nil {
		return err
	}
	attempt, closed := beginHostShutdown(ownerCtx, lifetime)
	if closed {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := completion.Wait(waitCtx, attempt.done); err != nil {
		return err
	}
	return attempt.err
}

func beginHostShutdown(
	ownerCtx context.Context,
	lifetime *hostLifetime,
) (attempt *hostShutdownAttempt, closed bool) {
	lifetime.closeMu.Lock()
	defer lifetime.closeMu.Unlock()
	if lifetime.closed {
		return nil, true
	}
	attempt = lifetime.shutdown
	if attempt == nil || attempt.completed {
		attempt = &hostShutdownAttempt{done: make(chan struct{})}
		lifetime.shutdown = attempt
		go runHostShutdown(ownerCtx, lifetime, attempt)
	}
	return attempt, false
}

func awaitHostShutdown(ctx context.Context, host *Host) error {
	if host == nil || host.lifetime == nil {
		return nil
	}
	attempt, closed := beginHostShutdown(ctx, host.lifetime)
	if closed {
		return nil
	}
	if err := completion.Wait(ctx, attempt.done); err != nil {
		return err
	}
	return attempt.err
}

func runHostShutdown(
	ownerCtx context.Context,
	lifetime *hostLifetime,
	attempt *hostShutdownAttempt,
) {
	components := []shutdownComponent{
		lifetime.goalDriver,
		lifetime.mcpCoordinator,
		lifetime.runCoordinator,
	}
	lifetime.closeMu.Lock()
	begin := !lifetime.stopping
	if begin {
		lifetime.stopping = true
	}
	lifetime.closeMu.Unlock()

	if begin {
		for _, component := range components {
			if component != nil {
				component.BeginShutdown()
			}
		}
	}

	var errs []error
	for _, component := range components {
		if component != nil {
			errs = append(errs, component.AwaitShutdown(ownerCtx))
		}
	}
	if lifetime.runEffectTasks != nil {
		// Run producers are stopped and joined before this point, so no new
		// terminal maintenance can be admitted. Let already accepted best-effort
		// work (notably Session title generation) settle naturally for a bounded
		// window; a stuck provider is then canceled through the normal owner.
		drainCtx, cancelDrain := context.WithTimeout(ownerCtx, runEffectDrainTimeout)
		_ = lifetime.runEffectTasks.Drain(drainCtx)
		cancelDrain()
		lifetime.runEffectTasks.Cancel()
		errs = append(errs, lifetime.runEffectTasks.Wait(ownerCtx))
	}
	if componentErr := errors.Join(errs...); componentErr != nil {
		finishHostShutdown(lifetime, attempt, false, componentErr)
		return
	}

	if lifetime.executor != nil {
		lifetime.executor.BeginShutdown()
		if err := lifetime.executor.AwaitShutdown(ownerCtx); err != nil {
			finishHostShutdown(lifetime, attempt, false, err)
			return
		}
	}
	lifetime.closeMu.Lock()
	if lifetime.resourceGraph == nil {
		// host resources are acquired before tool resources; concatenating them
		// in creation order lets the Sequence own the whole reverse dependency
		// graph in one self-continuing generation.
		steps := make([]*teardown.Step, 0, len(lifetime.hostResources)+len(lifetime.toolResources))
		steps = append(steps, lifetime.hostResources...)
		steps = append(steps, lifetime.toolResources...)
		lifetime.resourceGraph = teardown.NewSequence(steps)
	}
	resourceGraph := lifetime.resourceGraph
	lifetime.closeMu.Unlock()
	settled, resourceErr := resourceGraph.Shutdown(ownerCtx)
	if !settled {
		finishHostShutdown(lifetime, attempt, false, resourceErr)
		return
	}
	finishHostShutdown(lifetime, attempt, true, resourceErr)
}

func finishHostShutdown(
	lifetime *hostLifetime,
	attempt *hostShutdownAttempt,
	closed bool,
	err error,
) {
	lifetime.closeMu.Lock()
	defer lifetime.closeMu.Unlock()
	attempt.err = err
	attempt.completed = true
	if closed {
		lifetime.toolResources = nil
		lifetime.hostResources = nil
		lifetime.closed = true
	}
	close(attempt.done)
}
