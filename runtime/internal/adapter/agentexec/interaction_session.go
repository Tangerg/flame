package agentexec

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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
	otelagent "github.com/Tangerg/scope/otel/agent"
)

type interactionSession struct {
	executionTrees runs.ExecutionTreeStore
	ref            runs.ExecutorRef
	scope          runs.ExecutionScope
	deployment     agent.Deployment
	input          agent.Payload
	engine         *agent.Engine

	lifetime            interactionLifetime
	state               interactionState
	childProjection     interactionChildProjection
	accounting          interactionAccounting
	unknownPollInterval time.Duration
	statePollInterval   time.Duration
	mcpToolAutoApproved func(server, tool string) bool
	maintenance         RunMaintenance
	lifecycleHooks      InteractionLifecycleHooks
	buildID             runtimeidentity.BuildID
	start               runs.RootExecutionStart
	modelFailures       interactionModelFailures
	effectFailures      interactionEffectFailures
	segmentClock        interactionSegmentClock
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
	toolMetadata               map[string]toolResultMetadata
	mu                         sync.Mutex
	pendingSteers              map[agent.SignalID]pendingInteractionSteer
	pendingContinuation        *pendingInteractionContinuation
	process                    *agent.Process
	admittedProcessID          agent.ProcessID
	observerWasAttached        bool
	begun                      bool
	workersStarted             bool
	finished                   bool
	boundary                   interactionBoundary
	dispatchReady              chan struct{}
	waitingCheckpoint          runs.ExecutorCheckpoint
	subtreeChange              *interactionWaitingSubtreeChange
	subtreePrepared            chan struct{}
	unknownReported            bool
	deployments                *interactionDeploymentSet
	delegateCalls              map[delegateCallIdentity]*managedDelegateCall
	delegateChildren           map[agent.ProcessID]*managedDelegateCall
	durableContextWasCompacted bool
}

type pendingInteractionSteer struct {
	itemID  string
	content []transcript.ContentBlock
}

// pendingInteractionContinuation is Runtime-owned input that already has a
// durable transcript Item but cannot share Scope Interaction's input-response
// Signal. The model-context boundary applies it exactly once to the root
// conversation after the answered Tool result.
type pendingInteractionContinuation struct {
	processID agent.ProcessID
	itemID    string
	content   []transcript.ContentBlock
}

type interactionBoundary uint8

const (
	interactionBoundaryInactive interactionBoundary = iota
	interactionBoundaryWaiting
	interactionBoundaryContinuationStaged
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
	buildID runtimeidentity.BuildID,
	policy interactionExecutionPolicy,
	telemetry *otelagent.Observer,
) *interactionSession {
	return &interactionSession{
		telemetry:      telemetry,
		executionTrees: config.ExecutionTrees,
		ref:            ref, scope: rootExecutionScope(start), lifetime: newInteractionLifetime(lifetime),
		state: interactionState{
			pendingSteers:    make(map[agent.SignalID]pendingInteractionSteer),
			delegateCalls:    make(map[delegateCallIdentity]*managedDelegateCall),
			delegateChildren: make(map[agent.ProcessID]*managedDelegateCall),
		},
		modelFailures: newInteractionModelFailures(),
		accounting: newInteractionAccounting(
			start.ModelSelection,
			config.Pricing,
		),
		unknownPollInterval: policy.unknownEffectPollInterval,
		statePollInterval:   policy.statePollInterval,
		mcpToolAutoApproved: config.MCPToolAutoApproved,
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

func (i *interactionState) attachObserver() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.observerWasAttached || i.finished {
		return false
	}
	if i.begun && i.boundary != interactionBoundaryContinuationStaged &&
		i.boundary != interactionBoundarySubtreePrepared {
		return false
	}
	i.observerWasAttached = true
	return true
}

func (i *interactionState) detachObserver() {
	i.mu.Lock()
	i.observerWasAttached = false
	i.mu.Unlock()
}

func (i *interactionState) observerAttached() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.observerWasAttached
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
	if i.begun || i.finished {
		return false
	}
	i.begun = true
	return true
}

func (i *interactionState) setProcess(process *agent.Process) {
	i.mu.Lock()
	i.process = process
	i.mu.Unlock()
}

func (i *interactionState) processHandle() *agent.Process {
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
	i.lifetime.start(i.awaitGuarded, i.reconcileUnknownEffectsGuarded, i.reconcileExecutionStateGuarded)
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
	slog.ErrorContext(i.lifetime.execution, "agentexec: execution projection panicked",
		"session.id", i.start.SessionID, "panic", fmt.Sprint(recovered),
		"stack", string(debug.Stack()))
	i.publishProjectionFailure(fmt.Errorf("agentexec: execution projection panicked: %v", recovered))
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
		i.state.finished = true
		i.state.mu.Unlock()
		i.lifetime.stopExecution()
		i.stopReconciliation()
		close(i.lifetime.events)
		close(i.lifetime.done)
	})
}

// projectDelta forwards the preview text a client renders before the durable
// Item exists. Every reason a delta cannot be forwarded leaves the same hole in
// that preview, so every reason reports it: a payload this Runtime cannot parse
// is a framework boundary defect, not a quiet no-op.
func (i *interactionSession) projectDelta(ctx context.Context, delta agent.Delta) {
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
var errUnroutableDelta = errors.New("agentexec: delta has no accepting executor route")

func (i *interactionSession) reportDroppedDelta(ctx context.Context, delta agent.Delta, cause error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(
		"agentexec.delta.dropped",
		trace.WithAttributes(
			attribute.String("process.id", delta.ProcessID().String()),
			attribute.String("drop.cause", cause.Error()),
		),
	)
	if !errors.Is(cause, errUnroutableDelta) {
		span.RecordError(cause)
	}
}

func (i *interactionSession) flushDeltas(ctx context.Context) error {
	if i.engine == nil {
		return errors.New("agentexec: Interaction engine is unavailable")
	}
	if err := i.engine.FlushDeltas(ctx); err != nil {
		return fmt.Errorf("agentexec: flush model deltas: %w", err)
	}
	return nil
}

func (i *interactionSession) observeFrameworkEvent(_ context.Context, event agent.Event) {
	if event.Relation().RootID() != i.processRootID() {
		return
	}
	i.lifetime.wakeState()
}

func (i *interactionSession) processRootID() agent.ProcessID {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.process == nil {
		return agent.ProcessID{}
	}
	return i.state.process.Relation().RootID()
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
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(owner, cancel)
	defer func() {
		stop()
		cancel()
	}()
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
	if process == nil || i.state.finished || i.state.boundary != interactionBoundaryInactive {
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
	if i.state.finished || i.state.boundary != interactionBoundaryInactive ||
		i.state.process != process {
		i.state.mu.Unlock()
		return false
	}
	i.state.boundary = interactionBoundaryWaiting
	i.state.dispatchReady = make(chan struct{})
	i.state.waitingCheckpoint = checkpoint.Clone()
	i.state.mu.Unlock()
	published := i.lifetime.send(runs.ExecutorEvent{
		Member:  i.executorMember(process.Relation()),
		Payload: barrier,
	})
	if published && i.lifecycleHooks != nil {
		if err := i.lifecycleHooks.NotifyWaiting(
			i.lifetime.execution, i.start.SessionID, i.start.WorkspaceCWD,
		); err != nil {
			slog.ErrorContext(i.lifetime.execution, "agentexec: notify waiting",
				"session.id", i.start.SessionID, "error", err,
			)
		}
	}
	return published
}

func (i *interactionSession) stageContinuation(checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.finished || i.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if i.state.boundary != interactionBoundaryWaiting || i.state.observerWasAttached {
		return runs.ErrExecutionClaimed
	}
	if !executorCheckpointsEqual(i.state.waitingCheckpoint, checkpoint) {
		return fmt.Errorf("%w: live Interaction checkpoint differs from the claimed waiting boundary", runs.ErrInvalidExecutorCheckpoint)
	}
	i.state.boundary = interactionBoundaryContinuationStaged
	return nil
}

func (i *interactionSession) beginContinuation(allowedInterrupts []interrupt.Kind) error {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.finished || i.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if i.state.boundary != interactionBoundaryContinuationStaged || !i.state.observerWasAttached {
		return errors.New("agentexec: Interaction continuation was not staged and observed")
	}
	if !slices.Equal(i.start.InterruptKinds, allowedInterrupts) {
		return errors.New("agentexec: continuation capabilities differ from the staged Interaction")
	}
	return nil
}

func isInteractionWaitingBoundary(status agent.Status) bool {
	return status == agent.StatusWaiting || status == agent.StatusPaused
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
		left.Scope == right.Scope && left.ModelSelection.Equal(right.ModelSelection) &&
		slices.Equal(left.Usage.Models, right.Usage.Models) &&
		bytes.Equal(left.Payload, right.Payload)
}

func (i *interactionSession) reportUnknownEffects() bool {
	ctx := i.lifetime.reconciling
	ids, readable := i.unknownEffectIDs(ctx)
	if !readable || len(ids) == 0 {
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
	if err := process.Kill(ctx, unresolvedEffectsStopReason); err != nil && !errors.Is(err, agent.ErrProcessFinished) {
		i.publishProjectionFailure(err)
	}
	return true
}

func (i *interactionSession) await() {
	joinCtx, cancelJoin := i.lifetime.publicationContext(i.lifetime.execution)
	defer cancelJoin()
	result, err := i.state.process.Await(joinCtx)
	if joinErr := i.state.process.Join(joinCtx); joinErr != nil {
		err = errors.Join(err, joinErr)
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

func (i *interactionSession) publishResult(result agent.Result) error {
	member := runs.ExecutorMember{MemberID: result.ProcessID().String()}
	if result.Status() == agent.StatusCompleted {
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
			ctx, i.start.SessionID, i.start.WorkspaceCWD, string(end.Reason),
		); err != nil {
			slog.ErrorContext(i.lifetime.execution, "agentexec: notify stopped",
				"session.id", i.start.SessionID, "error", err,
			)
		}
	}
	return nil
}

func (i *interactionSession) publishProjectionFailure(cause error) {
	var stopped *agent.RuntimeError
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
		Detail: executorDiagnostic(cause),
	}
	if failure.Detail == "" {
		failure.Detail = "executor result could not be projected"
	}
	i.lifetime.send(runs.ExecutorEvent{
		Member:  member,
		Payload: runs.NewSegmentEnded(run.OutcomeFailed, &failure, nil, 0),
	})
}

func (i *interactionSession) release(ctx context.Context) error {
	i.lifetime.beginRelease()
	if err := i.discardPreparedSubtree(ctx); err != nil {
		return fmt.Errorf("agentexec: discard prepared waiting subtree before release: %w", err)
	}
	i.state.mu.Lock()
	process := i.state.process
	workersStarted := i.state.workersStarted
	i.state.mu.Unlock()
	if process != nil {
		var stopped *agent.RuntimeError
		if err := process.Kill(ctx, interactionReleaseReason); err != nil && !errors.Is(err, agent.ErrProcessFinished) && !errors.As(err, &stopped) {
			return fmt.Errorf("agentexec: kill Interaction execution: %w", err)
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
	// Projection may have failed before await joined the tree. Its completion
	// cannot stand in for Scope's proof that descendants have stopped.
	if process != nil {
		var stopped *agent.RuntimeError
		if err := process.Join(ctx); err != nil && !errors.As(err, &stopped) {
			return fmt.Errorf("agentexec: join Interaction execution: %w", err)
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
	if err := i.engine.Close(ctx); err != nil {
		return err
	}
	i.closeTelemetry()
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	return deployments.close()
}

// closeTelemetry ends any span the Engine left open. It is idempotent so the
// discard path can release an assembly that never reached closeExecution.
func (i *interactionSession) closeTelemetry() {
	if i.telemetry != nil {
		i.telemetry.Close()
	}
}

func (i *interactionSession) segmentEnd(result agent.Result) (runs.SegmentEnded, error) {
	termination := result.Termination()
	duration := i.segmentClock.duration(result.StartedAt(), result.FinishedAt())
	end := segmentEndFromTermination(termination, duration)
	if termination.Cause() == agent.TerminationCauseHostCancellation && termination.Reason() == modelProcessStopReason {
		if classified, found := i.modelFailures.lookup(result.ProcessID()); found {
			end.reason = run.OutcomeFailed
			end.failure = &classified
		}
	}
	usage, err := i.accounting.segmentUsage(result.ProcessID())
	if err != nil {
		return runs.SegmentEnded{}, err
	}
	ctx, cancel := i.lifetime.publicationContext(i.lifetime.execution)
	defer cancel()
	effects, err := i.terminalEffects(ctx, result.ProcessID())
	if err != nil {
		return runs.SegmentEnded{}, err
	}
	return runs.NewSegmentEnded(end.reason, end.failure, usage, end.duration).WithUnresolvedEffects(effects), nil
}

type segmentEndDraft struct {
	reason   run.Outcome
	failure  *run.Failure
	duration time.Duration
}

func segmentEndFromTermination(termination agent.Termination, duration time.Duration) segmentEndDraft {
	end := segmentEndDraft{duration: duration}
	switch termination.Cause() {
	case agent.TerminationCauseCompletion:
		end.reason = run.OutcomeCompleted
	case agent.TerminationCauseParentDeadline,
		agent.TerminationCauseHostDeadline:
		end.reason = run.OutcomeTimedOut
		failure := run.Failure{
			Kind:   run.FailureTimeout,
			Detail: "executor deadline reached",
		}
		end.failure = &failure
	case agent.TerminationCauseParentCancellation, agent.TerminationCauseHostCancellation:
		end.reason = run.OutcomeCanceled
	case agent.TerminationCauseExecutionFailure:
		failure, _ := termination.Failure()
		end.reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureAgentStuck,
			Detail: executorDiagnostic(errors.New(failure.Message())),
		}
		end.failure = &problem
	case agent.TerminationCauseExternalFailure:
		end.reason = run.OutcomeFailed
		failure, _ := termination.Failure()
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: executorDiagnostic(errors.New(failure.Message())),
		}
		switch failure.Code() {
		case "interaction.model.failed":
			problem.Kind = run.FailureProviderUnavailable
		case "interaction.model.invalid_response", "interaction.model.tool_calls_not_completed":
			problem.Kind = run.FailureProviderRejected
		case "interaction.delegate.unresolved_effects":
			end.reason = run.OutcomeLost
			problem.Kind = run.FailureLost
		}
		end.failure = &problem
	case agent.TerminationCauseContractFailure, agent.TerminationCausePanic:
		end.reason = run.OutcomeFailed
		failure, _ := termination.Failure()
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: executorDiagnostic(errors.New(failure.Message())),
		}
		end.failure = &problem
	case agent.TerminationCauseEngineKill:
		if termination.Reason() == unresolvedEffectsStopReason {
			end.reason = run.OutcomeLost
			end.failure = &run.Failure{Kind: run.FailureLost, Detail: "external operations have no provable durable result"}
			break
		}
		end.reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: termination.Reason(),
		}
		end.failure = &problem
	default:
		end.reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: "executor returned an unknown terminal cause",
		}
		end.failure = &problem
	}
	return end
}

const unresolvedEffectsStopReason = "runtime unresolved external effects"
