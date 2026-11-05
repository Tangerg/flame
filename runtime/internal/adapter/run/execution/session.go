package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/keylock"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	otelagent "github.com/Tangerg/scope/otel/agent"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type interactionSession struct {
	executionTrees runs.ExecutionTreeStore
	toolResults    originalToolResults
	ref            runs.ExecutorRef
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
	accounting             interactionAccounting
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
// and Delegate topology. Accounting and Segment timing have independent
// invariants and do not belong under this lock.
type interactionState struct {
	restoreTree                scoperun.Log
	toolMetadata               map[string]toolResultMetadata
	mu                         sync.Mutex
	pendingSteers              map[scoperun.SignalID]runs.PreparedSteer
	pendingContinuation        *pendingInteractionContinuation
	process                    *scoperun.Handle[scoperun.Payload]
	admittedProcessID          scoperun.ProcessID
	observer                   context.Context
	phase                      interactionPhase
	workersStarted             bool
	boundary                   interactionBoundary
	dispatchReady              chan struct{}
	waitingCheckpoint          runs.ExecutorCheckpoint
	subtreeChange              *interactionWaitingSubtreeChange
	subtreePrepared            chan struct{}
	unknownReported            bool
	deployments                *interactionDeploymentSet
	delegateCalls              map[delegateCallIdentity]*managedDelegateCall
	delegateChildren           map[scoperun.ProcessID]*managedDelegateCall
	durableContextWasCompacted bool
}

// pendingInteractionContinuation is Runtime-owned input that already has a
// durable transcript Item but cannot share Scope Interaction's input-response
// Signal. The model-context boundary applies it exactly once to the root
// conversation after the answered Tool result.
type pendingInteractionContinuation struct {
	processID scoperun.ProcessID
	itemID    string
	content   []transcript.ContentBlock
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
	interactionBoundaryAcceptedPaused
	interactionBoundarySubtreePreparing
	interactionBoundarySubtreePrepared
	interactionBoundarySubtreeApplying
	interactionBoundarySubtreeApplied
	interactionBoundarySubtreeRecovery
)

// Scope's Interaction reports these external failures only by code
// string; Scope does not export them, so this is Flame's single copy.
const (
	failureInvalidModelResponse  = "interaction.model.invalid_response"
	failureToolCallsNotCompleted = "interaction.model.tool_calls_not_completed"
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
		state: interactionState{
			pendingSteers:    make(map[scoperun.SignalID]runs.PreparedSteer),
			delegateCalls:    make(map[delegateCallIdentity]*managedDelegateCall),
			delegateChildren: make(map[scoperun.ProcessID]*managedDelegateCall),
		},
		modelFailures:       newInteractionModelFailures(),
		modelEvidence:       config.ModelEvidence,
		callComposition:     config.CallComposition,
		accounting:          newInteractionAccounting(start.ModelSelection),
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
	if i.phase == interactionBegun && i.boundary != interactionBoundaryAcceptedPaused &&
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

func (i *interactionState) markDurableContextCompacted() {
	i.mu.Lock()
	i.durableContextWasCompacted = true
	i.mu.Unlock()
}

func (i *interactionState) durableContextCompacted() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.durableContextWasCompacted
}

func (i *interactionState) begin() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.phase != interactionPending {
		return false
	}
	i.phase = interactionBegun
	return true
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

func (i *interactionSession) reconcileUnknownEffectsGuarded() {
	defer i.reportExecutionDefect()
	i.reconcileUnknownEffects()
}

func (i *interactionSession) reconcileExecutionStateGuarded() {
	defer i.reportExecutionDefect()
	i.reconcileExecutionState()
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

func (i *interactionSession) stopReconciliation() {
	i.lifetime.stopReconciling()
	i.lifetime.reconcilers.Wait()
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

// projectDelta forwards the preview text a client renders before the durable
// Item exists. Every reason a delta cannot be forwarded leaves the same hole in
// that preview, so every reason reports it: a payload this Runtime cannot parse
// is a framework boundary defect, not a quiet no-op.
func (i *interactionSession) projectDelta(ctx context.Context, delta scoperun.Delta) {
	parsed, err := interaction.ParseModelResponseDelta(delta.Payload())
	if err != nil {
		i.reportDroppedDelta(ctx, delta, err)
		return
	}
	response := parsed.ResponseDelta()
	for _, part := range response.Parts {
		var payload runs.ExecutionFact
		switch part.Kind {
		case corechat.PartDeltaText, corechat.PartDeltaRefusal:
			payload = runs.MessageDelta{Text: part.Text}
		case corechat.PartDeltaReasoning:
			payload = runs.ReasoningDelta{Text: part.Text}
		default:
			continue
		}
		member, found := i.executorMemberByProcessID(delta.ProcessID())
		if found && i.lifetime.offer(runs.ExecutorEvent{Member: member, Payload: payload}) {
			continue
		}
		i.reportDroppedDelta(ctx, delta, errUnroutableDelta)
	}
}

// errUnroutableDelta names the drop that is not a defect: the process has no
// installed executor route, or the run tree stopped accepting events.
var errUnroutableDelta = errors.New("execution: delta has no accepting executor route")

func (i *interactionSession) reportDroppedDelta(ctx context.Context, delta scoperun.Delta, cause error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(
		"execution.delta.dropped",
		trace.WithAttributes(
			attribute.String("process.id", delta.ProcessID().String()),
			attribute.String("drop.cause", cause.Error()),
		),
	)
	if !errors.Is(cause, errUnroutableDelta) {
		span.RecordError(cause)
		slog.ErrorContext(ctx, "execution: model response delta dropped",
			"process.id", delta.ProcessID().String(), "error", cause)
	}
}

// Delta delivery is a host-owned preview channel. Flush asks its sole reader to
// finish every already-emitted preview before the durable result is published.
func (i *interactionSession) flushDeltas(ctx context.Context) error {
	done := make(chan struct{})
	select {
	case i.deltaFlush <- done:
	case <-ctx.Done():
		return ctx.Err()
	case <-i.lifetime.releasing.Done():
		return errInteractionReleased
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-i.lifetime.releasing.Done():
		return errInteractionReleased
	}
}

func (i *interactionSession) consumeDeltas() {
	ctx := i.lifetime.releasing
	for {
		select {
		case delta := <-i.deltas:
			i.projectDelta(ctx, delta)
		case done := <-i.deltaFlush:
			draining := true
			for draining {
				select {
				case delta := <-i.deltas:
					i.projectDelta(ctx, delta)
				default:
					draining = false
				}
			}
			close(done)
		case <-ctx.Done():
			return
		}
	}
}

func (i *interactionSession) consumeTelemetry() {
	process := i.state.processHandle()
	if process == nil {
		return
	}
	if _, err := i.telemetry.Consume(i.lifetime.releasing, process, 0); err != nil && i.lifetime.releasing.Err() == nil {
		slog.ErrorContext(i.lifetime.releasing, "execution: observe Interaction log", "error", err)
	}
}

func (i *interactionSession) processRootID() scoperun.ProcessID {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.process == nil {
		return i.state.admittedProcessID
	}
	return i.state.process.ID().Root()
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

func (i *interactionSession) reconcileUnknownEffects() {
	ticker := time.NewTicker(i.unknownPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-i.lifetime.unknownWake:
		case <-ticker.C:
		case <-i.lifetime.reconciling.Done():
			return
		}
		if i.reportUnknownEffects() {
			return
		}
	}
}

func (i *interactionSession) reconcileExecutionState() {
	ticker := time.NewTicker(i.statePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-i.lifetime.stateWake:
		case <-ticker.C:
		case <-i.lifetime.reconciling.Done():
			return
		}
		progressed, err := i.reconcileCompletedDelegateChildren(i.lifetime.reconciling)
		if err != nil {
			if !releasedDuringProjection(err) {
				i.publishProjectionFailure(err)
			}
			return
		}
		if progressed {
			continue
		}
		if i.publishWaitingBoundary() {
			continue
		}
	}
}

func (i *interactionSession) publishWaitingBoundary() bool {
	i.state.mu.Lock()
	process := i.state.process
	if process == nil || i.state.phase == interactionFinished || i.state.boundary != interactionBoundaryInactive {
		i.state.mu.Unlock()
		return false
	}
	i.state.mu.Unlock()
	ctx := i.lifetime.reconciling
	snapshot, interruptions, found, err := i.captureHumanInputBarrier(ctx)
	if err != nil {
		if !releasedDuringProjection(err) {
			i.publishProjectionFailure(err)
		}
		return false
	}
	if !found {
		return false
	}
	// Capture can settle a sibling that was still running at the preflight.
	// Its product terminal must precede the waiting members from this cut.
	if _, err := i.reconcileCompletedDelegateChildren(ctx); err != nil {
		if !releasedDuringProjection(err) {
			i.publishProjectionFailure(err)
		}
		return false
	}
	checkpoint, err := i.executorCheckpoint(snapshot)
	if err != nil {
		i.publishProjectionFailure(err)
		return false
	}
	barrier, err := runs.NewTreeInterrupted(checkpoint, interruptions)
	if err != nil {
		i.publishProjectionFailure(err)
		return false
	}
	i.state.mu.Lock()
	// The captured cut is the authority on what was waiting; re-reading a live
	// status here would describe a different moment than the checkpoint does.
	if i.state.phase == interactionFinished || i.state.boundary != interactionBoundaryInactive ||
		i.state.process != process {
		i.state.mu.Unlock()
		return false
	}
	i.state.boundary = interactionBoundaryWaiting
	i.state.dispatchReady = make(chan struct{})
	i.state.waitingCheckpoint = checkpoint.Clone()
	i.state.mu.Unlock()
	published := i.lifetime.send(runs.ExecutorEvent{
		Member:  i.executorMember(process.ID()),
		Payload: barrier,
	})
	if published && i.lifecycleHooks != nil {
		if err := i.lifecycleHooks.NotifyWaiting(
			i.lifetime.execution, i.start.SessionID, i.start.WorkspaceCWD,
		); err != nil {
			slog.ErrorContext(i.lifetime.execution, "execution: notify waiting",
				"session.id", i.start.SessionID, "error", err,
			)
		}
	}
	return published
}

func (i *interactionSession) stageAcceptedContinuation(checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.phase == interactionFinished || i.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if i.state.boundary != interactionBoundaryWaiting || i.state.observer != nil {
		return runs.ErrExecutionClaimed
	}
	if !executorCheckpointsEqual(i.state.waitingCheckpoint, checkpoint) {
		return fmt.Errorf("%w: live Interaction checkpoint differs from the claimed waiting boundary", runs.ErrInvalidExecutorCheckpoint)
	}
	i.state.boundary = interactionBoundaryAcceptedPaused
	return nil
}

func (i *interactionSession) beginContinuation() error {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.phase == interactionFinished || i.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if i.state.boundary != interactionBoundaryAcceptedPaused || i.state.observer == nil {
		return errors.New("execution: Interaction continuation was not staged and observed")
	}
	return nil
}

func isInteractionWaitingBoundary(status scoperun.Status) bool {
	return status == scoperun.StatusWaiting || status == scoperun.StatusPaused
}

func (i *interactionSession) continuationAccepted() {
	i.state.mu.Lock()
	i.state.continueExecution()
	i.state.mu.Unlock()
}

// continueExecution releases Effects woken by child cancellation only after the
// replacement product Segment owns their projections. The caller holds mu.
func (i *interactionState) continueExecution() {
	i.boundary = interactionBoundaryInactive
	i.waitingCheckpoint = runs.ExecutorCheckpoint{}
	close(i.dispatchReady)
	i.dispatchReady = nil
}

func executorCheckpointsEqual(left, right runs.ExecutorCheckpoint) bool {
	return slices.Equal(left.ToolResultIDs, right.ToolResultIDs) && left.RootMemberID == right.RootMemberID && left.BuildID == right.BuildID &&
		left.SessionID == right.SessionID &&
		bytes.Equal(left.Payload, right.Payload)
}

func (i *interactionSession) reportUnknownEffects() bool {
	ctx := i.lifetime.reconciling
	if ctx.Err() != nil {
		return true
	}
	ids, err := i.unknownEffectIDs(ctx)
	if err != nil || len(ids) == 0 {
		return false
	}
	resumeDispatch := i.state.holdReceiptInvestigation()
	if resumeDispatch == nil {
		return true
	}
	defer resumeDispatch()
	if _, err := i.investigateToolReceipts(ctx); err != nil {
		if ctx.Err() != nil || releasedDuringProjection(err) {
			return true
		}
		i.publishProjectionFailure(err)
		return true
	}
	ids, err = i.unknownEffectIDs(ctx)
	if err != nil {
		if ctx.Err() != nil || releasedDuringProjection(err) {
			return true
		}
		i.publishProjectionFailure(err)
		return true
	}
	if len(ids) == 0 {
		return false
	}
	i.state.mu.Lock()
	if i.state.unknownReported {
		i.state.mu.Unlock()
		return true
	}
	i.state.unknownReported = true
	process := i.state.process
	i.state.mu.Unlock()
	// Unknown effects need execution to stop, but cannot override cancellation
	// already accepted by Scope. Both causes enter its ordered control queue;
	// the immutable result decides which cause became authoritative.
	if err := process.Cancel(ctx, unresolvedEffectsStopReason); err != nil &&
		!errors.Is(err, scoperun.ErrProcessFinished) && !releasedDuringProjection(err) {
		i.publishProjectionFailure(err)
	}
	// A control request cannot resolve uncertainty. Stop the writer and let
	// Await report its retained effects; the product owner decides loss.
	if err := i.engine.Close(ctx); err != nil && !releasedDuringProjection(err) {
		i.publishProjectionFailure(err)
	}
	return true
}

// A receipt may resolve an Effect before its result projection is complete.
// Only the investigation that installed this gate can release model dispatch.
func (i *interactionState) holdReceiptInvestigation() func() {
	i.mu.Lock()
	if i.unknownReported {
		i.mu.Unlock()
		return nil
	}
	var gate chan struct{}
	if i.dispatchReady == nil {
		gate = make(chan struct{})
		i.dispatchReady = gate
	}
	i.mu.Unlock()
	return func() {
		if gate != nil {
			i.mu.Lock()
			if i.dispatchReady == gate {
				close(gate)
				i.dispatchReady = nil
			}
			i.mu.Unlock()
		}
	}
}

func (i *interactionSession) await() {
	joinCtx, cancelJoin := i.lifetime.publicationContext(i.lifetime.execution)
	defer cancelJoin()
	result, err := i.state.process.Await(joinCtx)
	// Await owns the committed Exit. Close joins all local dispatch work when
	// the writer instead stopped with unresolved effects or a durability error.
	if err != nil {
		err = errors.Join(err, i.engine.Close(joinCtx))
	}
	i.stopReconciliation()
	if err == nil {
		_, err = i.reconcileCompletedDelegateChildren(joinCtx)
	}
	if err == nil {
		err = i.publishResult(result)
	}
	if err == nil {
		err = i.closeExecution(joinCtx)
	}
	if err != nil {
		i.publishProjectionFailure(err)
	}
	i.finish()
}

func (i *interactionSession) publishResult(result scoperun.Result[scoperun.Payload]) error {
	member := runs.ExecutorMember{MemberID: result.ProcessID().String()}
	if result.Termination().Status() == scoperun.StatusCompleted {
		i.maintainCompletedRoot()
	}
	end, err := i.segmentEnd(result)
	if err != nil {
		return err
	}
	ctx := context.WithoutCancel(i.lifetime.execution)
	if err := i.commitFact(ctx, member, end); err != nil {
		return err
	}
	i.modelFailures.forget(result.ProcessID())
	if i.lifecycleHooks != nil {
		ctx, cancel := context.WithTimeout(ctx, auxiliaryOperationTimeout)
		defer cancel()
		if err := i.lifecycleHooks.NotifyStopped(
			ctx, i.start.SessionID, i.start.WorkspaceCWD, string(end.Ending.Outcome()),
		); err != nil {
			slog.ErrorContext(i.lifetime.execution, "execution: notify stopped",
				"session.id", i.start.SessionID, "error", err,
			)
		}
	}
	return nil
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
	member := runs.ExecutorMember{}
	i.state.mu.Lock()
	if i.state.admittedProcessID.Valid() {
		member.MemberID = i.state.admittedProcessID.String()
	}
	i.state.mu.Unlock()
	failure := run.Failure{
		Kind:   run.FailureInternal,
		Detail: run.Diagnostic(cause),
	}
	if failure.Detail == "" {
		failure.Detail = "executor result could not be projected"
	}
	i.lifetime.send(runs.ExecutorEvent{
		Member:  member,
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

func (i *interactionSession) segmentEnd(result scoperun.Result[scoperun.Payload]) (runs.SegmentEnded, error) {
	termination := result.Termination()
	duration := i.segmentClock.duration(result.StartedAt(), result.FinishedAt())
	end := i.segmentEndForOwner(result.ProcessID(), termination, duration)
	ctx, cancel := i.lifetime.publicationContext(i.lifetime.execution)
	defer cancel()
	effects, err := i.terminalEffects(ctx, result.ProcessID())
	if err != nil {
		return runs.SegmentEnded{}, err
	}
	return runs.NewSegmentEnded(end.ending, end.duration).WithUnresolvedEffects(effects), nil
}

func (i *interactionSession) segmentEndForOwner(owner scoperun.ProcessID, termination scoperun.Termination, duration time.Duration) segmentEndDraft {
	end := segmentEndFromTermination(termination, duration)
	if termination.Status() == scoperun.StatusFailed ||
		termination.Cause() == scoperun.TerminationCauseHostCancellation && termination.Reason() == modelProcessStopReason {
		if classified, found := i.modelFailures.lookup(owner); found {
			end.ending = run.Failed{Failure: classified}
		}
	}
	return end
}

type segmentEndDraft struct {
	ending   run.Ending
	duration time.Duration
}

func segmentEndFromTermination(termination scoperun.Termination, duration time.Duration) segmentEndDraft {
	return segmentEndDraft{ending: terminationEnding(termination), duration: duration}
}

func terminationEnding(termination scoperun.Termination) run.Ending {
	failed := func(kind run.FailureKind, detail string) run.Ending {
		return run.Failed{Failure: run.Failure{Kind: kind, Detail: detail}}
	}
	switch termination.Cause() {
	case scoperun.TerminationCauseCompletion:
		return run.Completed{}
	case scoperun.TerminationCauseParentDeadline, scoperun.TerminationCauseHostDeadline:
		return run.TimedOut{Detail: "executor deadline reached"}
	case scoperun.TerminationCauseParentCancellation, scoperun.TerminationCauseHostCancellation:
		if termination.Cause() == scoperun.TerminationCauseHostCancellation && termination.Reason() == unresolvedEffectsStopReason {
			return run.Lost{Loss: run.Loss{Reason: run.LossUnresolvedEffects}}
		}
		return run.Canceled{}
	case scoperun.TerminationCauseExecutionFailure:
		failure, _ := termination.Failure()
		return failed(run.FailureAgentStuck, run.Diagnostic(errors.New(failure.Message())))
	case scoperun.TerminationCauseExternalFailure:
		failure, _ := termination.Failure()
		detail := run.Diagnostic(errors.New(failure.Message()))
		switch failure.Code() {
		case failureInvalidModelResponse, failureToolCallsNotCompleted:
			return failed(run.FailureProviderRejected, detail)
		}
		return failed(run.FailureInternal, detail)
	case scoperun.TerminationCauseContractFailure, scoperun.TerminationCausePanic:
		failure, _ := termination.Failure()
		return failed(run.FailureInternal, run.Diagnostic(errors.New(failure.Message())))
	case scoperun.TerminationCauseRuntimeKill:
		return failed(run.FailureInternal, termination.Reason())
	default:
		return failed(run.FailureInternal, "executor returned an unknown terminal cause")
	}
}

const unresolvedEffectsStopReason = "runtime unresolved external effects"
