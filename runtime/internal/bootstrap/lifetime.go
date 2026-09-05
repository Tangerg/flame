package bootstrap

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/infra/process/teardown"
)

type runtimeLifetime struct {
	delivery            *delivery.Endpoint
	stopRuntime         context.CancelFunc
	schedulerDone       <-chan struct{}
	databaseChangesDone <-chan struct{}
	recoveryDone        <-chan struct{}

	context      context.Context
	closeMu      sync.Mutex
	stopping     bool
	closed       bool
	shutdownWait shutdownWaitPolicy
	shutdown     *shutdownAttempt

	goalDriver     shutdownComponent
	mcpCoordinator shutdownComponent
	runCoordinator shutdownComponent
	executor       shutdownComponent
	runEffectTasks taskOwner
	toolResources  []*teardown.Step
	hostResources  []*teardown.Step
	resourceGraph  *teardown.Sequence
}

func newRuntimeLifetime(ctx context.Context, resources []TerminalResource) *runtimeLifetime {
	return &runtimeLifetime{
		context:       ctx,
		shutdownWait:  defaultShutdownWaitPolicy(),
		hostResources: terminalResources(resources),
	}
}

type shutdownAttempt struct {
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

// Close shuts the complete Runtime down in reverse dependency order.
// The first caller starts one shutdown attempt that stops
// producers, drains then cancels accepted maintenance, joins components, and
// finally enters terminal resource teardown.
// Each caller has a bounded wait, but its deadline cannot abandon or duplicate
// that generation. A completed component error permits a later Close to start
// one new generation; terminal resource diagnostics close the graph. Idempotent
// across Instance copies once the graph has fully closed.
func (i *Instance) Close() error {
	if i == nil || i.lifetime == nil {
		return nil
	}
	return closeRuntimeLifetime(i.lifetime)
}

func closeRuntimeLifetime(lifetime *runtimeLifetime) error {
	if lifetime == nil {
		return nil
	}
	// Preserve instance trace values, but never let the caller that happened to
	// start Close cancel the owner generation. A nil lifetime context occurs only
	// in direct Instance tests and uses the same process-owner root as the wait.
	ownerCtx := context.Background()
	if lifetime.context != nil {
		ownerCtx = context.WithoutCancel(lifetime.context)
	}
	timeout, err := shutdownWaitTimeout(lifetime.shutdownWait)
	if err != nil {
		return err
	}
	attempt, closed := beginShutdown(ownerCtx, lifetime)
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

func beginShutdown(
	ownerCtx context.Context,
	lifetime *runtimeLifetime,
) (attempt *shutdownAttempt, closed bool) {
	lifetime.closeMu.Lock()
	defer lifetime.closeMu.Unlock()
	if lifetime.closed {
		return nil, true
	}
	attempt = lifetime.shutdown
	if attempt == nil || attempt.completed {
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
		lifetime.runCoordinator,
	}
	lifetime.closeMu.Lock()
	begin := !lifetime.stopping
	if begin {
		lifetime.stopping = true
	}
	lifetime.closeMu.Unlock()

	if begin {
		lifetime.delivery.BeginShutdown()
		if lifetime.stopRuntime != nil {
			lifetime.stopRuntime()
		}
	}
	if err := lifetime.delivery.AwaitShutdown(ownerCtx); err != nil {
		finishShutdown(lifetime, attempt, false, err)
		return
	}
	for _, done := range []<-chan struct{}{lifetime.schedulerDone, lifetime.databaseChangesDone, lifetime.recoveryDone} {
		if err := completion.Wait(ownerCtx, done); err != nil {
			finishShutdown(lifetime, attempt, false, err)
			return
		}
	}
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
		finishShutdown(lifetime, attempt, false, componentErr)
		return
	}

	if lifetime.executor != nil {
		lifetime.executor.BeginShutdown()
		if err := lifetime.executor.AwaitShutdown(ownerCtx); err != nil {
			finishShutdown(lifetime, attempt, false, err)
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
		finishShutdown(lifetime, attempt, false, resourceErr)
		return
	}
	finishShutdown(lifetime, attempt, true, resourceErr)
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
	attempt.completed = true
	if closed {
		lifetime.toolResources = nil
		lifetime.hostResources = nil
		lifetime.closed = true
	}
	close(attempt.done)
}
