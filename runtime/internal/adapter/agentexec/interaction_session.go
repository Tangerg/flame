package agentexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

type interactionSession struct {
	ref        runs.ExecutorRef
	scope      runs.ExecutionScope
	deployment agent.Deployment
	input      agent.Input
	engine     *agent.Engine

	lifetime            interactionLifetime
	state               interactionState
	childProjection     interactionChildProjection
	accounting          interactionAccounting
	allowance           *interactionAllowance
	unknownPollInterval time.Duration
	statePollInterval   time.Duration
	mcpToolAutoApproved func(server, tool string) bool
	maintenance         RunMaintenance
	lifecycleHooks      InteractionLifecycleHooks
	buildID             runtimeidentity.BuildID
	start               runs.RootExecutionStart
	toolOutcomes        interactionToolOutcomes
	modelFailures       interactionModelFailures
	committedReplies    interactionCommittedReplies
	segmentClock        interactionSegmentClock
}

// interactionState owns the one lock domain whose facts must move atomically:
// the live Process, its observation/waiting boundary, exact pending steers,
// Delegate topology, and cancellation plane. Accounting, repetition detection,
// committed replies, and Segment timing have independent invariants and do not
// belong under this lock.
type interactionState struct {
	mu                         sync.Mutex
	pendingSteers              map[agent.SignalID]pendingInteractionSteer
	pendingContinuation        *pendingInteractionContinuation
	process                    *agent.Process
	admittedProcessID          agent.ProcessID
	observerWasAttached        bool
	begun                      bool
	finished                   bool
	boundary                   interactionBoundary
	waitingCheckpoint          runs.ExecutorCheckpoint
	subtreeChange              *interactionWaitingSubtreeChange
	subtreePrepared            chan struct{}
	unknownReported            bool
	deployments                *interactionDeploymentSet
	delegateCalls              map[delegateCallIdentity]*managedDelegateCall
	delegateChildren           map[agent.ProcessID]*managedDelegateCall
	activeDispatches           map[interactionDispatchIdentity]activeInteractionDispatch
	canceledSubtreeRoots       map[agent.ProcessID]struct{}
	rootCancellationRequested  bool
	durableContextWasCompacted bool
}

type activeInteractionDispatch struct {
	processID agent.ProcessID
	cancel    context.CancelCauseFunc
}

type pendingInteractionSteer struct {
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
) *interactionSession {
	return &interactionSession{
		ref: ref, scope: rootExecutionScope(start), lifetime: newInteractionLifetime(lifetime),
		state: interactionState{
			pendingSteers:        make(map[agent.SignalID]pendingInteractionSteer),
			delegateCalls:        make(map[delegateCallIdentity]*managedDelegateCall),
			delegateChildren:     make(map[agent.ProcessID]*managedDelegateCall),
			activeDispatches:     make(map[interactionDispatchIdentity]activeInteractionDispatch),
			canceledSubtreeRoots: make(map[agent.ProcessID]struct{}),
		},
		committedReplies: newInteractionCommittedReplies(),
		modelFailures:    newInteractionModelFailures(),
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
	i.lifetime.start(i.await, i.reconcileUnknownEffects, i.reconcileExecutionState)
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

func (i *interactionSession) projectDelta(ctx context.Context, delta agent.Delta) {
	parsed, err := interaction.ParseModelResponseDelta(delta.Payload())
	if err != nil {
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
		trace.SpanFromContext(ctx).AddEvent(
			"agentexec.delta.dropped",
			trace.WithAttributes(attribute.String("process.id", delta.ProcessID().String())),
		)
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
	ctx, cancel := i.lifetime.bind(ctx)
	defer cancel()
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
		ctx, cancel := context.WithTimeout(i.lifetime.reconciling, authoritativeProjectionTimeout)
		progressed, err := i.reconcileCompletedDelegateChildren(ctx)
		cancel()
		if err != nil {
			i.publishProjectionFailure(err)
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
	if process.Status() != agent.StatusWaiting {
		return false
	}
	ctx, cancel := context.WithTimeout(i.lifetime.reconciling, authoritativeProjectionTimeout)
	defer cancel()
	snapshot, interruptions, found, err := i.captureHumanInputBarrier(ctx)
	if err != nil {
		i.publishProjectionFailure(err)
		return false
	}
	if !found {
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
	if i.state.finished || i.state.boundary != interactionBoundaryInactive ||
		i.state.process != process || process.Status() != agent.StatusWaiting {
		i.state.mu.Unlock()
		return false
	}
	i.state.boundary = interactionBoundaryWaiting
	i.state.waitingCheckpoint = checkpoint.Clone()
	i.state.mu.Unlock()
	published := i.lifetime.send(runs.ExecutorEvent{
		Member:  i.executorMember(process.Relation()),
		Payload: barrier,
	})
	if published && i.lifecycleHooks != nil {
		i.lifecycleHooks.NotifyWaiting(
			i.lifetime.execution, i.start.SessionID, i.start.CWD,
		)
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
	if i.state.boundary != interactionBoundaryWaiting || i.state.observerWasAttached ||
		!isInteractionWaitingBoundary(i.state.process.Status()) {
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
	if i.state.boundary != interactionBoundaryContinuationStaged || !i.state.observerWasAttached ||
		!isInteractionWaitingBoundary(i.state.process.Status()) {
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
	i.state.boundary = interactionBoundaryInactive
	i.state.waitingCheckpoint = runs.ExecutorCheckpoint{}
	i.state.mu.Unlock()
}

func executorCheckpointsEqual(left, right runs.ExecutorCheckpoint) bool {
	return left.RootMemberID == right.RootMemberID && left.BuildID == right.BuildID &&
		left.Scope == right.Scope && left.ModelSelection.Equal(right.ModelSelection) &&
		left.Limits == right.Limits && slices.Equal(left.Usage.Models, right.Usage.Models) &&
		bytes.Equal(left.Payload, right.Payload)
}

func (i *interactionSession) reportUnknownEffects() bool {
	ctx, cancel := context.WithTimeout(i.lifetime.reconciling, authoritativeProjectionTimeout)
	defer cancel()
	ids, err := i.unknownEffectIDs(ctx)
	if err != nil || len(ids) == 0 {
		return false
	}
	i.state.mu.Lock()
	if i.state.unknownReported {
		i.state.mu.Unlock()
		return true
	}
	i.state.unknownReported = true
	member := runs.ExecutorMember{MemberID: i.state.process.Relation().ProcessID().String()}
	i.state.mu.Unlock()
	return i.lifetime.send(runs.ExecutorEvent{
		Member: member, Payload: runs.NewUnknownEffectsDetected(),
	})
}

func (i *interactionSession) await() {
	joinCtx := context.WithoutCancel(i.lifetime.execution)
	result, err := i.state.process.Await(joinCtx)
	if err == nil {
		projectionCtx, cancel := context.WithTimeout(joinCtx, authoritativeProjectionTimeout)
		_, err = i.reconcileCompletedDelegateChildren(projectionCtx)
		cancel()
	}
	i.stopReconciliation()
	if err == nil {
		err = i.engine.Close()
	}
	if err == nil {
		err = i.publishResult(result)
	}
	if err != nil {
		i.publishProjectionFailure(err)
	}
	i.finish()
}

func (i *interactionSession) publishResult(result agent.Result) error {
	member := runs.ExecutorMember{MemberID: result.ProcessID().String()}
	if result.Status() == agent.StatusCompleted {
		erased, ok := result.Output()
		if !ok {
			return errors.New("agentexec: completed Interaction has no output")
		}
		output, err := erased.Decode[interaction.Output]()
		if err != nil {
			return fmt.Errorf("decode Interaction output: %w", err)
		}
		if output.Source != interaction.CompletionSourceModelResponse || output.ModelResponse == nil {
			return fmt.Errorf("unsupported Interaction completion source %q", output.Source)
		}
		modelOutput := output.ModelResponse.Output
		if modelOutput == nil || modelOutput.Message == nil {
			return errors.New("agentexec: Interaction output has no assistant message")
		}
		completion, err := runs.NewAssistantMessageCompleted(*modelOutput.Message)
		if err != nil {
			return err
		}
		if !i.lifetime.send(runs.ExecutorEvent{
			Member: member, Payload: completion,
		}) {
			return nil
		}
		i.maintainCompletedRoot()
	}
	end, err := i.segmentEnd(result)
	if err != nil {
		return err
	}
	i.lifetime.send(runs.ExecutorEvent{Member: member, Payload: end})
	if i.lifecycleHooks != nil {
		i.lifecycleHooks.NotifyStopped(
			i.lifetime.execution, i.start.SessionID, i.start.CWD, string(end.Reason),
		)
	}
	return nil
}

func (i *interactionSession) publishProjectionFailure(cause error) {
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
		Payload: runs.SegmentEnded{Reason: run.OutcomeFailed, Failure: &failure},
	})
}

func (i *interactionSession) release(ctx context.Context) error {
	i.lifetime.beginRelease()
	if err := i.discardPreparedSubtree(ctx); err != nil {
		return fmt.Errorf("agentexec: discard prepared waiting subtree before release: %w", err)
	}
	i.state.mu.Lock()
	process := i.state.process
	begun := i.state.begun
	finished := i.state.finished
	i.state.mu.Unlock()
	if !begun {
		i.failStart()
		return i.engine.Close()
	}
	if process != nil && !finished {
		if err := process.Kill(ctx, interactionReleaseReason); err != nil && !errors.Is(err, agent.ErrProcessFinished) {
			return fmt.Errorf("agentexec: kill Interaction execution: %w", err)
		}
	}
	select {
	case <-i.lifetime.done:
		i.lifetime.workers.Wait()
		return i.engine.Close()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *interactionSession) segmentEnd(result agent.Result) (runs.SegmentEnded, error) {
	termination := result.Termination()
	duration := i.segmentClock.duration(result.StartedAt(), result.FinishedAt())
	// The model Effect and Scope's host-context watcher observe the same owner
	// cancellation concurrently. If the model returns context.Canceled first,
	// Scope can freeze that Effect as an external failure before its watcher
	// records host cancellation. The Runtime lifetime is the authoritative fact
	// at this boundary, so do not project that scheduling race as provider
	// failure. Other framework terminal causes remain untouched.
	ownerCause := i.lifetime.ownerCause()
	var end runs.SegmentEnded
	if termination.Cause() == agent.TerminationCauseExternalFailure && ownerCause != nil {
		end = segmentEndFromOwnerCause(ownerCause, duration)
	} else if stop := i.allowance.terminal(); stop != interactionAllowanceOpen {
		end = segmentEndFromAllowance(stop, duration)
	} else {
		end = segmentEndFromTermination(termination, duration)
		if termination.Cause() == agent.TerminationCauseExternalFailure {
			failure, _ := termination.Failure()
			if failure.Code() == "interaction.model.failed" {
				if classified, found := i.modelFailures.take(result.ProcessID()); found {
					end.Failure = &classified
				}
			}
		}
	}
	usage, err := i.accounting.segmentUsage(result.ProcessID())
	if err != nil {
		return runs.SegmentEnded{}, err
	}
	end.Usage = usage
	return end, nil
}

func segmentEndFromAllowance(stop interactionAllowanceStop, duration time.Duration) runs.SegmentEnded {
	end := runs.SegmentEnded{Duration: duration}
	switch stop {
	case interactionAllowanceStepsExhausted:
		end.Reason = run.OutcomeMaxSteps
	case interactionAllowanceBudgetExhausted:
		end.Reason = run.OutcomeMaxBudget
	case interactionAllowancePricingUnavailable:
		end.Reason = run.OutcomeFailed
		end.Failure = &run.Failure{
			Kind:   run.FailureProviderRejected,
			Detail: "served model pricing is unavailable for the configured cost limit",
		}
	default:
		panic("agentexec: impossible allowance stop")
	}
	return end
}

func segmentEndFromOwnerCause(cause error, duration time.Duration) runs.SegmentEnded {
	end := runs.SegmentEnded{Reason: run.OutcomeCanceled, Duration: duration}
	if errors.Is(cause, context.DeadlineExceeded) {
		end.Reason = run.OutcomeTimedOut
		end.Failure = &run.Failure{
			Kind:   run.FailureTimeout,
			Detail: "executor deadline reached",
		}
	}
	return end
}

func segmentEndFromTermination(termination agent.Termination, duration time.Duration) runs.SegmentEnded {
	end := runs.SegmentEnded{Duration: duration}
	switch termination.Cause() {
	case agent.TerminationCauseCompletion:
		end.Reason = run.OutcomeCompleted
	case agent.TerminationCauseProcessDeadline,
		agent.TerminationCauseParentDeadline,
		agent.TerminationCauseHostDeadline:
		end.Reason = run.OutcomeTimedOut
		failure := run.Failure{
			Kind:   run.FailureTimeout,
			Detail: "executor deadline reached",
		}
		end.Failure = &failure
	case agent.TerminationCauseParentCancellation, agent.TerminationCauseHostCancellation:
		end.Reason = run.OutcomeCanceled
	case agent.TerminationCauseExecutionFailure:
		failure, _ := termination.Failure()
		if failure.Code() == "interaction.limit.model_calls" {
			end.Reason = run.OutcomeMaxSteps
			break
		}
		end.Reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureAgentStuck,
			Detail: executorDiagnostic(errors.New(failure.Message())),
		}
		end.Failure = &problem
	case agent.TerminationCauseExternalFailure:
		end.Reason = run.OutcomeFailed
		failure, _ := termination.Failure()
		if failure.Code() == "interaction.host.failed" {
			problem := run.Failure{
				Kind:   run.FailureInternal,
				Detail: executorDiagnostic(errors.New(failure.Message())),
			}
			end.Failure = &problem
			break
		}
		detail := executorDiagnostic(errors.New(failure.Message()))
		if detail == "" {
			detail = "model provider failed"
		}
		problem := run.Failure{
			Kind:   run.FailureProviderUnavailable,
			Detail: detail,
		}
		end.Failure = &problem
	case agent.TerminationCauseContractFailure, agent.TerminationCausePanic:
		end.Reason = run.OutcomeFailed
		failure, _ := termination.Failure()
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: executorDiagnostic(errors.New(failure.Message())),
		}
		end.Failure = &problem
	case agent.TerminationCauseEngineKill:
		end.Reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: termination.Reason(),
		}
		end.Failure = &problem
	default:
		end.Reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: "executor returned an unknown terminal cause",
		}
		end.Failure = &problem
	}
	return end
}
