package agentexec

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestInteractionExecutorAppliesColdWaitingDelegateCancellationWithoutDuplicateProjection(
	t *testing.T,
) {
	fixture := newWaitingDelegateFixture(t, "interaction-waiting-cancellation-test")
	started := fixture.start(t)
	initialEventsReady := make(chan []runs.Event, 1)
	go func() { initialEventsReady <- slices.Collect(started.Events) }()
	select {
	case events := <-initialEventsReady:
		if len(events) == 0 {
			t.Fatal("waiting Delegate produced no events")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waiting Delegate did not park")
	}
	barrier := fixture.waitForBarrier(t, time.Second)
	pending := barrier.Pending()
	if len(pending.Interrupts) != 1 || len(pending.Continuations) != 2 {
		t.Fatalf("waiting Delegate boundary = %#v", barrier)
	}
	ref := runs.ExecutorRef{SessionID: pending.SessionID, ExecutorID: pending.ExecutorID}
	if err := fixture.executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	target := pending.Interrupts[0]
	targetMemberID := memberIDForRun(t, pending, target.RunID)
	continuation := waitingDelegateContinuation(barrier)
	request, err := runs.NewWaitingSubtreeCancellationRequest(
		continuation,
		targetMemberID,
		"caller canceled the waiting delegate",
	)
	if err != nil {
		t.Fatal(err)
	}
	continuation.Members[0].MemberID = "member_changed"
	continuation.Checkpoint.Payload[0] = 'x'
	continuation.Capabilities.InterruptKinds[0] = "changed"
	projected := request.Continuation()
	projected.Members[0].MemberID = "member_projected"
	projected.Checkpoint.Payload[0] = 'y'
	projected.Capabilities.InterruptKinds[0] = "projected"
	prepareCtx, cancelPrepare := context.WithTimeout(t.Context(), 2*time.Second)
	prepared, err := fixture.executor.PrepareWaitingSubtreeCancellation(prepareCtx, request)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	if validateErr := prepared.Validate(); validateErr != nil {
		cancelPrepare()
		t.Fatal(validateErr)
	}
	cancelPrepare()
	if discardErr := prepared.Discard(); discardErr != nil {
		t.Fatal(discardErr)
	}
	liveSession, err := fixture.executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	liveSession.state.mu.Lock()
	boundaryAfterDiscard := liveSession.state.boundary
	observerAfterDiscard := liveSession.state.observerWasAttached
	statusAfterDiscard := liveSession.state.process.Status()
	liveSession.state.mu.Unlock()
	assertDiscardedWaitingCancellationState(
		t,
		boundaryAfterDiscard,
		observerAfterDiscard,
		isInteractionWaitingBoundary(statusAfterDiscard),
		statusAfterDiscard.String(),
	)

	prepareCtx, cancelPrepare = context.WithTimeout(t.Context(), 2*time.Second)
	prepared, err = fixture.executor.PrepareWaitingSubtreeCancellation(prepareCtx, request)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	assertPreparedWaitingCancellation(t, prepared, targetMemberID, cancelPrepare)
	sequence, err := fixture.executor.Observe(context.Background(), ref)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	resumedEvents := collectInteractionEvents(sequence)
	if err := prepared.Apply(runs.WaitingSubtreeResumesRunning); err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	if calls := fixture.model.Calls(); calls != 2 {
		cancelPrepare()
		t.Fatalf("provider calls after state apply = %d, want 2 before continuation activation", calls)
	}
	if err := prepared.Continue(t.Context()); err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	cancelPrepare()
	var observed []runs.ExecutorEvent
	select {
	case observed = <-resumedEvents:
	case <-time.After(3 * time.Second):
		t.Fatal("root did not finish after waiting Delegate cancellation")
	}
	assertCanceledDelegateIsNotReprojected(t, observed, targetMemberID)
	if fixture.model.Calls() != 3 {
		t.Fatalf("provider calls = %d, want 3 without canceled-child replay", fixture.model.Calls())
	}
	if err := fixture.executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	fixture.shutdown(t)
}

func assertDiscardedWaitingCancellationState(
	t *testing.T,
	boundary interactionBoundary,
	observerAttached bool,
	waitingStatus bool,
	statusDescription string,
) {
	t.Helper()
	if boundary != interactionBoundaryWaiting || observerAttached || !waitingStatus {
		t.Fatalf(
			"discarded subtree boundary=%d observer=%t status=%s",
			boundary, observerAttached, statusDescription,
		)
	}
}

func assertPreparedWaitingCancellation(
	t *testing.T,
	prepared runs.PreparedWaitingSubtreeCancellation,
	targetMemberID string,
	cancelPrepare context.CancelFunc,
) {
	t.Helper()
	canceledMemberIDs := prepared.CanceledMemberIDs()
	pausedMemberIDs := prepared.PausedMemberIDs()
	checkpoint := prepared.Checkpoint()
	pendingInterruptions := prepared.PendingInterruptions()
	if len(canceledMemberIDs) == 1 &&
		canceledMemberIDs[0] == targetMemberID &&
		len(pausedMemberIDs) == 1 &&
		pausedMemberIDs[0] == checkpoint.RootMemberID &&
		len(pendingInterruptions) == 0 {
		return
	}
	cancelPrepare()
	t.Fatalf(
		"prepared waiting cancellation canceled=%v paused=%v interruptions=%d",
		canceledMemberIDs,
		pausedMemberIDs,
		len(pendingInterruptions),
	)
}

func assertCanceledDelegateIsNotReprojected(
	t *testing.T,
	events []runs.ExecutorEvent,
	canceledMemberID string,
) {
	t.Helper()
	for _, event := range events {
		if event.Member.MemberID == canceledMemberID {
			t.Fatalf("canceled Delegate leaked a duplicate executor projection: %#v", event)
		}
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted {
		t.Fatalf("terminal events = %#v, want one completed root", ended)
	}
}

func memberIDForRun(t *testing.T, pending runs.Pending, runID string) string {
	t.Helper()
	for _, continuation := range pending.Continuations {
		if continuation.RunID == runID {
			return continuation.MemberID
		}
	}
	t.Fatalf("Run %q has no waiting member", runID)
	return ""
}

func waitingDelegateContinuation(barrier runs.TreeBarrierCommit) runs.WaitingContinuation {
	pending := barrier.Pending()
	members := make([]runs.WaitingMember, 0, len(pending.Continuations))
	for _, member := range pending.Continuations {
		members = append(members, runs.WaitingMember{
			RunID: member.RunID, MemberID: member.MemberID,
			ParentRunID: member.Lineage.ParentRunID, SpawnedByItemID: member.Lineage.SpawnedByItemID,
			ModelSelection: member.ModelSelection, Metrics: member.Metrics,
			DrainedTools: slices.Clone(member.DrainedTools),
		})
	}
	return runs.WaitingContinuation{
		SessionID: pending.SessionID, ExecutorID: pending.ExecutorID,
		RootRunID: pending.RootRunID, Members: members,
		Checkpoint: barrier.Checkpoint(), Capabilities: pending.Capabilities,
		ChildRunAdmissionEnabled: pending.Capabilities.ChildRuns,
	}
}
