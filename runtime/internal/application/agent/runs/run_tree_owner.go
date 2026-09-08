package runs

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// runCleanupTimeout bounds request-detached Run teardown, so a stuck store or
// executor cannot wedge cancellation.
const runCleanupTimeout = 5 * time.Second

// runTreeOwner is the root Segment's process-local ownership record. It holds the
// lifecycle join, event journal, immutable executor bindings and the root-owned
// cancellation arbiter; specialized behavior lives beside those concerns.
type runTreeOwner struct {
	mu              sync.Mutex
	cancel          context.CancelFunc
	taskContext     context.Context
	hub             *journal
	done            chan struct{}
	completionErr   error
	terminalRun     *run.Run
	terminalRuns    map[string]run.Run
	executorMembers map[string]string
	childCancel     *childCancellation
	cancelRequested bool
	cancelReason    string
	activation      segmentActivation
	interrupt       interruptBoundary
}

// segmentActivation serializes the post-commit executor activation with root
// cancellation. The durable Run becomes Running before BeginExecution crosses
// the executor side-effect boundary, so cancellation must either win before
// activation starts or wait for activation to finish; executing both calls at
// once leaves neither side able to classify the resulting executor error.
type segmentActivation struct {
	done     chan struct{}
	started  bool
	finished bool
	err      error
}

// newRunTreeOwner builds the root Segment's complete ownership record. Its
// cancellation, task context, journal, completion barrier and activation gate
// all exist before any caller can reach it, so no method has to repair a
// half-built owner or answer for one that was never built.
func newRunTreeOwner(cancel context.CancelFunc, taskContext context.Context, hub *journal) *runTreeOwner {
	return &runTreeOwner{
		cancel:          cancel,
		taskContext:     taskContext,
		hub:             hub,
		done:            make(chan struct{}),
		activation:      segmentActivation{done: make(chan struct{})},
		executorMembers: make(map[string]string),
	}
}

// beginExecution crosses the executor activation boundary exactly once. A root
// cancellation that claimed the owner first suppresses activation and lets the
// pump synthesize the canonical canceled terminal from the committed opening.
func (r *runTreeOwner) beginExecution(
	ctx context.Context,
	begin func(context.Context) error,
) (canceled bool, err error) {
	r.mu.Lock()
	if r.activation.started || r.activation.finished {
		r.mu.Unlock()
		return false, errors.New("runs: segment activation already resolved")
	}
	if r.cancelRequested {
		r.activation.finished = true
		close(r.activation.done)
		r.mu.Unlock()
		return true, nil
	}
	r.activation.started = true
	r.mu.Unlock()

	if begin != nil {
		err = begin(ctx)
	}
	r.mu.Lock()
	r.activation.err = err
	r.activation.finished = true
	close(r.activation.done)
	r.mu.Unlock()
	return false, err
}

func (r *runTreeOwner) rejectActivation(cause error) error {
	if cause == nil {
		return errors.New("runs: activation rejection requires a cause")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activation.started || r.activation.finished {
		return errors.New("runs: segment activation already resolved")
	}
	r.activation.err = cause
	r.activation.finished = true
	close(r.activation.done)
	return nil
}

func (r *runTreeOwner) committedTerminalRun() (run.Run, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminalRun == nil {
		return run.Run{}, false
	}
	return *r.terminalRun, true
}

// stop cancels the run context. Called on a true terminal (never on a parked
// Run, whose live executor must stay alive for resume).
func (r *runTreeOwner) stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	cancel()
}

// wait joins the complete run boundary: terminal projection, registry removal,
// synchronous maintenance, admission release, and journal closure.
func (r *runTreeOwner) wait(ctx context.Context) error {
	if err := completion.Wait(ctx, r.done); err != nil {
		return err
	}
	return r.completionErr
}

// runCleanupContext bounds a run's durable cancel and never inherits the
// caller's cancellation: teardown has to outlive the request that asked for it.
func runCleanupContext(base context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(base), runCleanupTimeout)
}

// cleanupContext roots teardown on the run's detached owner context. The pump
// can release — and cancel — its task owner immediately after requestCancel
// stops runCtx, so cleanup keeps the owner's trace values without inheriting
// that lifecycle cancellation.
func (r *runTreeOwner) cleanupContext() (context.Context, context.CancelFunc) {
	r.mu.Lock()
	taskContext := r.taskContext
	r.mu.Unlock()
	return runCleanupContext(taskContext)
}
