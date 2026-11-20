package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/keylock"

	scoperun "github.com/Tangerg/scope/agent/run"
	otelagent "github.com/Tangerg/scope/otel/agent"
)

type interactionSession struct {
	executionTrees runs.ExecutionTreeStore
	toolResults    originalToolResults
	ref            runs.ExecutorRef
	rootID         scoperun.ProcessID
	scope          runs.ExecutionScope
	deployment     scoperun.Program[scoperun.Payload, scoperun.Payload]
	input          scoperun.Payload
	engine         *scoperun.Runtime
	deltas         chan scoperun.Delta
	deltaFlush     chan chan struct{}
	logCache       executionLogCache

	lifetime               interactionLifetime
	state                  interactionState
	steerAdmissions        *keylock.Set
	childProjection        interactionChildProjection
	material               requestMaterial
	unknownPollInterval    time.Duration
	statePollInterval      time.Duration
	maintenance            RunMaintenance
	lifecycleHooks         interactionLifecycleHooks
	buildID                runs.BuildID
	start                  runs.RootExecutionStart
	modelFailures          interactionModelFailures
	modelObservers         []plugins.ContributionBinding
	compositionActivations []plugin.Activation
	modelContexts          []plugins.ContributionBinding
	modelInputs            []plugins.ContributionBinding
	toolProposals          []plugins.ContributionBinding
	selectedChat           modeladapter.SelectedChat
	modelEvidence          interactionModelEvidence
	callComposition        interactionCallComposition
	effectFailures         interactionEffectFailures
	segmentClock           interactionSegmentClock
	// telemetry observes Framework facts only. It owns Process, Step, and
	// Effect spans plus the commit instruments, and must outlive the Engine it
	// observes, so it closes after the Engine has drained.
	telemetry *otelagent.Observer
}

// interactionState owns the one lock domain whose facts must move atomically:
// the live Process, its observation/waiting boundary, exact pending steers,
// and Delegate topology. Request material and Segment timing have independent
// invariants and do not belong under this lock.
type interactionState struct {
	restoreTree   scoperun.Log
	toolMetadata  map[string]toolCallMetadata
	mu            sync.Mutex
	pendingSteers map[scoperun.SignalID]runs.PreparedSteer
	// Resume Items are already committed, but the root model can still be
	// waiting for delegated work across several successive input barriers.
	pendingResumeInputs []runs.CommittedUserInput
	process             *scoperun.Handle[scoperun.Payload]
	observer            context.Context
	phase               interactionPhase
	workersStarted      bool
	boundary            interactionBoundary
	// Each gate holds model dispatch for one owner and is released only by it:
	// a crossed boundary until its replacement Segment owns projection, and a
	// receipt investigation until its result projection completes. The two run
	// on different goroutines, so one gate cannot stand in for the other.
	boundaryGate      chan struct{}
	investigationGate chan struct{}
	waitingCheckpoint runs.ExecutorCheckpoint
	subtreeChange     *interactionWaitingSubtreeChange
	subtreePrepared   chan struct{}
	unknownReported   bool
	deployments       *interactionDeploymentSet
	delegateCalls     map[delegateCallIdentity]*managedDelegateCall
	delegateChildren  map[scoperun.ProcessID]*managedDelegateCall
}

// interactionPhase orders one session's claim: a Process is begun at most once,
// and a finished session never begins.
type interactionPhase uint8

const (
	interactionPending interactionPhase = iota
	interactionBegun
	interactionFinished
)

type interactionBoundary uint8

const (
	interactionBoundaryInactive interactionBoundary = iota
	interactionBoundaryRecovery
	interactionBoundaryWaiting
	interactionBoundaryAccepted
	interactionBoundarySubtreePreparing
	interactionBoundarySubtreePrepared
	interactionBoundarySubtreeApplying
	interactionBoundarySubtreeApplied
	interactionBoundarySubtreeRecovery
)

func newInteractionSession(
	lifetime context.Context,
	ref runs.ExecutorRef,
	start runs.RootExecutionStart,
	config InteractionExecutorConfig,
	buildID runs.BuildID,
	policy interactionExecutionPolicy,
	telemetry *otelagent.Observer,
) *interactionSession {
	return &interactionSession{
		telemetry:       telemetry,
		deltaFlush:      make(chan chan struct{}),
		steerAdmissions: keylock.NewSet(),
		executionTrees:  config.ExecutionTrees,
		toolResults:     config.OriginalToolResults,
		ref:             ref, scope: rootExecutionScope(start), lifetime: newInteractionLifetime(lifetime),
		rootID: interactionStartID(start.SessionID, start.RunID).Root(),
		state: interactionState{
			pendingSteers:    make(map[scoperun.SignalID]runs.PreparedSteer),
			delegateCalls:    make(map[delegateCallIdentity]*managedDelegateCall),
			delegateChildren: make(map[scoperun.ProcessID]*managedDelegateCall),
		},
		modelFailures:       newInteractionModelFailures(),
		modelEvidence:       config.ModelEvidence,
		callComposition:     config.CallComposition,
		material:            newRequestMaterial(),
		unknownPollInterval: policy.unknownEffectPollInterval,
		statePollInterval:   policy.statePollInterval,
		maintenance:         config.Maintenance,
		lifecycleHooks:      config.LifecycleHooks,
		buildID:             buildID, start: start,
	}
}

func interactionSegmentDuration(
	processStartedAt time.Time,
	segmentStartedAt time.Time,
	finishedAt time.Time,
) time.Duration {
	startedAt := processStartedAt
	if segmentStartedAt.After(startedAt) {
		startedAt = segmentStartedAt
	}
	return max(finishedAt.Sub(startedAt), 0)
}

func (i *interactionState) attachObserver(ctx context.Context) (func(), bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.observer != nil || i.phase == interactionFinished {
		return nil, false
	}
	if i.phase == interactionBegun && i.boundary != interactionBoundaryAccepted &&
		i.boundary != interactionBoundarySubtreePrepared {
		return nil, false
	}
	observer, cancel := context.WithCancel(ctx)
	i.observer = observer
	return func() {
		cancel()
		i.mu.Lock()
		defer i.mu.Unlock()
		if i.observer == observer {
			i.observer = nil
		}
	}, true
}

func (i *interactionState) observerAttached() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.observer != nil
}

func (i *interactionState) observationContext() context.Context {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.observer
}

func (i *interactionState) begin(boundary interactionBoundary) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.phase != interactionPending {
		return errors.New("execution: Interaction execution was already begun")
	}
	if i.boundary != boundary || i.observer == nil {
		return errors.New("execution: Interaction must be staged for this boundary and observed before begin")
	}
	i.phase = interactionBegun
	return nil
}

func (i *interactionState) setProcess(process *scoperun.Handle[scoperun.Payload]) {
	i.mu.Lock()
	i.process = process
	i.mu.Unlock()
}

func (i *interactionState) processHandle() *scoperun.Handle[scoperun.Payload] {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.process
}

func (i *interactionSession) startWorkers() {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	select {
	case <-i.lifetime.releasing.Done():
		return
	default:
	}
	i.state.workersStarted = true
	i.lifetime.start(i.awaitGuarded, i.reconcileUnknownEffectsGuarded, i.reconcileExecutionStateGuarded, i.consumeDeltas, i.consumeTelemetry, i.followOwnerLifetime)
}

// Scope detaches admitted work from the Start caller. The session's process
// owner therefore stops the writer explicitly, and Close joins its callbacks.
func (i *interactionSession) followOwnerLifetime() {
	select {
	case <-i.lifetime.done:
		return
	case <-i.lifetime.execution.Done():
	}
	ctx, cancel := i.lifetime.publicationContext(context.Background())
	defer cancel()
	if err := i.engine.Close(ctx); err != nil && !releasedDuringProjection(err) {
		slog.ErrorContext(ctx, "execution: close Runtime after owner stopped", "error", err)
	}
}

// The session already owns "this projection cannot continue", so a defect in one
// execution fails that Run through the same boundary an unrecoverable projection
// error uses instead of unwinding out of a goroutine nothing can recover from.
// Only the awaiting worker may finish the session: a reconciler that tried would
// join itself.
func (i *interactionSession) awaitGuarded() {
	defer i.finishOnExecutionDefect()
	i.await()
}

// finishOnExecutionDefect and reportExecutionDefect must be deferred directly so
// recover() sees the panic.
func (i *interactionSession) finishOnExecutionDefect() {
	if i.recordExecutionDefect(recover()) {
		i.finish()
	}
}

func (i *interactionSession) reportExecutionDefect() {
	i.recordExecutionDefect(recover())
}

func (i *interactionSession) recordExecutionDefect(recovered any) bool {
	if recovered == nil {
		return false
	}
	// The stack cannot travel in the Run's failure and is the only thing that
	// fixes the defect, so it goes to the operator here.
	slog.ErrorContext(i.lifetime.execution, "execution: execution projection panicked",
		"session.id", i.start.SessionID, "panic", fmt.Sprint(recovered),
		"stack", string(debug.Stack()))
	i.publishProjectionFailure(fmt.Errorf("execution: execution projection panicked: %v", recovered))
	return true
}

func (i *interactionSession) failStart() {
	i.finish()
}

func (i *interactionSession) finish() {
	i.lifetime.finishOnce.Do(func() {
		i.state.mu.Lock()
		i.state.phase = interactionFinished
		i.state.mu.Unlock()
		i.lifetime.stopExecution()
		close(i.lifetime.done)
		i.stopReconciliation()
		i.lifetime.closePublication()
	})
}

func (i *interactionSession) commitFact(
	ctx context.Context,
	member runs.ExecutorMember,
	fact runs.ExecutionFact,
) error {
	// New work follows execution cancellation. Settled outcomes survive it until
	// the product owner releases the session and stops consuming publication.
	owner := i.lifetime.execution
	switch fact.(type) {
	case runs.ModelCallCompleted, runs.ModelCallFailed, runs.ToolResultsCommitted, runs.ExecutionTreeSettled,
		runs.SegmentEnded:
		owner = i.lifetime.releasing
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := owner.Err(); err != nil {
		return err
	}
	// A receipt belongs to its consuming Segment observer. Once that observer
	// retires, activation must stop waiting before the pump joins activation.
	observer := i.state.observationContext()
	if observer == nil {
		return errInteractionReleased
	}
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(owner, cancel)
	stopObservation := context.AfterFunc(observer, cancel)
	defer func() {
		stop()
		stopObservation()
		cancel()
	}()
	if observer.Err() != nil {
		cancel()
	}
	ctx = bound
	commit, receipt, err := runs.NewExecutionFactCommit(fact)
	if err != nil {
		return err
	}
	event := runs.ExecutorEvent{Member: member, Payload: commit}
	if err := i.lifetime.sendAuthoritative(ctx, event); err != nil {
		return err
	}
	return receipt.Await(ctx)
}

func (i *interactionSession) await() {
	joinCtx, cancelJoin := i.lifetime.publicationContext(i.lifetime.execution)
	defer cancelJoin()
	result, err := i.state.process.Await(joinCtx)
	// A successful Await follows the joint Exit and Run commit. Close joins
	// local dispatch work when no Exit could be acknowledged.
	if err != nil {
		err = errors.Join(err, i.engine.Close(joinCtx))
	}
	i.stopReconciliation()
	if err == nil {
		ending := i.endingForProcess(result.ProcessID(), result.Termination())
		i.modelFailures.forget(result.ProcessID())
		if ending.Outcome() == run.OutcomeCompleted {
			i.maintainCompletedRoot()
		}
		i.notifyStopped(ending.Outcome())
		if err := i.closeExecution(joinCtx); err != nil {
			slog.ErrorContext(joinCtx, "execution: close settled Interaction", "error", err)
		}
	} else {
		i.publishProjectionFailure(err)
	}
	i.finish()
}

func (i *interactionSession) notifyStopped(outcome run.Outcome) {
	if i.lifecycleHooks != nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(i.lifetime.execution), auxiliaryOperationTimeout)
		defer cancel()
		if err := i.lifecycleHooks.NotifyStopped(
			ctx, i.start.SessionID, i.start.WorkspaceCWD, string(outcome),
		); err != nil {
			slog.ErrorContext(i.lifetime.execution, "execution: notify stopped",
				"session.id", i.start.SessionID, "error", err,
			)
		}
	}
}

// reportProjectionFailure publishes a failure unless the session was released
// meanwhile; a release ends reconciliation without a fault.
func (i *interactionSession) reportProjectionFailure(err error) {
	if !releasedDuringProjection(err) {
		i.publishProjectionFailure(err)
	}
}

func (i *interactionSession) publishProjectionFailure(cause error) {
	var stopped *scoperun.RuntimeError
	if errors.As(cause, &stopped) {
		if err := i.publishRuntimeFailure(cause); err == nil {
			return
		} else {
			cause = errors.Join(cause, err)
		}
	}
	failure := run.Failure{
		Kind:   run.FailureInternal,
		Detail: run.Diagnostic(cause),
	}
	if failure.Detail == "" {
		failure.Detail = "executor result could not be projected"
	}
	i.lifetime.send(runs.ExecutorEvent{
		Member:  runs.ExecutorMember{MemberID: i.rootID.String()},
		Payload: runs.NewSegmentEnded(run.Failed{Failure: failure}, 0),
	})
}

func (i *interactionSession) release(ctx context.Context) error {
	i.lifetime.beginRelease()
	if err := i.discardPreparedSubtree(ctx); err != nil {
		return fmt.Errorf("execution: discard prepared waiting subtree before release: %w", err)
	}
	i.state.mu.Lock()
	workersStarted := i.state.workersStarted
	i.state.mu.Unlock()
	if i.engine != nil {
		if err := i.engine.Close(ctx); err != nil {
			return fmt.Errorf("execution: close Interaction Runtime: %w", err)
		}
	}
	if workersStarted {
		select {
		case <-i.lifetime.done:
			i.lifetime.workers.Wait()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if !workersStarted {
		i.finish()
	}
	return i.closeExecution(ctx)
}

// Scope must drain the Engine before the host revokes its Tool capabilities.
// An interrupted close retains those resources for the next release attempt.
func (i *interactionSession) closeExecution(ctx context.Context) error {
	if i.engine != nil {
		if err := i.engine.Close(ctx); err != nil {
			return err
		}
	}
	if err := i.closeTelemetry(ctx); err != nil {
		return err
	}
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	return errors.Join(deployments.close(), i.selectedChat.Close(context.WithoutCancel(ctx)))
}

// closeTelemetry ends any span the Engine left open. It is idempotent so the
// discard path can release an assembly that never reached closeExecution.
func (i *interactionSession) closeTelemetry(ctx context.Context) error {
	if i.telemetry != nil {
		return i.telemetry.Close(ctx)
	}
	return nil
}
