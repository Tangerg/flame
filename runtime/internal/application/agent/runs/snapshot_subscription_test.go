package runs

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type snapshotExecutor struct {
	fakeExecutor
	publish   chan struct{}
	attempted chan struct{}
}

func (e *snapshotExecutor) Observe(ctx context.Context, ref ExecutorRef) (iter.Seq[ExecutorEvent], error) {
	events, err := e.fakeExecutor.Observe(ctx, ref)
	return func(yield func(ExecutorEvent) bool) {
		select {
		case <-e.publish:
		case <-ctx.Done():
			return
		}
		close(e.attempted)
		events(yield)
	}, err
}

type snapshotEffects struct {
	fakeEffects
	committed chan struct{}
}

func (e *snapshotEffects) CommitEvent(ctx context.Context, commit EventCommit) error {
	select {
	case e.committed <- struct{}{}:
	default:
	}
	return e.fakeEffects.CommitEvent(ctx, commit)
}

func TestSnapshotSubscriptionHandsOffBeforeItemAndRootCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	executor := &snapshotExecutor{fakeExecutor: fakeExecutor{events: []ExecutorPayload{
		MessageDelta{Text: "review complete"}, SegmentEnded{Reason: run.OutcomeCompleted},
	}}, publish: make(chan struct{}), attempted: make(chan struct{})}
	effects := &snapshotEffects{committed: make(chan struct{}, 1)}
	coordinator := testCoordinator(executor, effects)
	spec := testSegment()
	coordinator.runs = &fakeRunProjection{runs: map[string]run.Run{spec.RunID: runForSegment(spec)}}
	if _, err := coordinator.openSegment(ctx, spec); err != nil {
		t.Fatal(err)
	}
	snapshotRead := false
	attached, err := coordinator.SubscribeSnapshot(ctx, SubscribeRequest{RunID: spec.RunID, SegmentID: spec.SegmentID}, func(_ context.Context, sessionID string) error {
		if sessionID != spec.SessionID {
			t.Fatalf("snapshot session = %s", sessionID)
		}
		close(executor.publish)
		<-executor.attempted
		select {
		case <-effects.committed:
			t.Error("Run committed after the snapshot read began but before its tail attached")
		case <-time.After(25 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		snapshotRead = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(attached.Events)
	completedItem, completedRun := false, false
	for _, event := range events {
		switch event.Payload.(type) {
		case ItemCompleted:
			completedItem = true
		case SegmentFinished:
			completedRun = true
		}
	}
	if !snapshotRead || !completedItem || !completedRun {
		t.Fatalf("snapshot=%v item completion=%v root completion=%v", snapshotRead, completedItem, completedRun)
	}
}

func TestSnapshotSubscriptionReadFailureReleasesPublication(t *testing.T) {
	coordinator, hub := liveCoordinator(t, runRecord(run.Running, testSegmentID, ""))
	failure := errors.New("snapshot unavailable")
	if _, err := coordinator.SubscribeSnapshot(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID}, func(context.Context, string) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("read error = %v", err)
	}
	attached, err := coordinator.SubscribeSnapshot(t.Context(), SubscribeRequest{RunID: testRunID, SegmentID: testSegmentID}, func(context.Context, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	mustAppendJournal(t, hub, ev(true))
	mustCloseJournal(t, hub)
	if events := collectEvents(attached.Events); len(events) != 1 {
		t.Fatalf("tail events = %d", len(events))
	}
}

func TestSnapshotSubscriptionIncludesChildOpeningAndCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	request, confirmation := newChildStartFixture(time.Now())
	root := ExecutorMember{MemberID: "member_root"}
	child := ExecutorMember{MemberID: "member_child", ParentID: root.MemberID, SpawnCallID: "delegate_source"}
	executor := &snapshotExecutor{fakeExecutor: fakeExecutor{executorEvents: []ExecutorEvent{
		{Member: root, Payload: ToolCallStarted{CallID: "delegate", SourceCallID: child.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`}},
		{Member: child, Payload: request},
		{Member: child, Payload: MessageDelta{Text: "child review"}},
		{Member: child, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
		{Member: root, Payload: ToolCallFinished{CallID: "delegate", OutputText: "child review"}},
		{Member: root, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
	}}, publish: make(chan struct{}), attempted: make(chan struct{})}
	coordinator := testCoordinator(executor, &fakeEffects{})
	coordinator.newRunID = func() string { return "run_child" }
	coordinator.newSegmentID = func() string { return "seg_child" }
	spec := testSegment()
	spec.Capabilities = run.Capabilities{ChildRuns: true}
	coordinator.runs = &fakeRunProjection{runs: map[string]run.Run{spec.RunID: runForSegment(spec)}}
	if _, err := coordinator.openSegment(ctx, spec); err != nil {
		t.Fatal(err)
	}
	attached, err := coordinator.SubscribeSnapshot(ctx, SubscribeRequest{RunID: spec.RunID, SegmentID: spec.SegmentID, CallerCapabilities: spec.Capabilities}, func(context.Context, string) error {
		close(executor.publish)
		<-executor.attempted
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(attached.Events)
	if _, err := confirmation.Await(ctx); err != nil {
		t.Fatal(err)
	}
	requirePublishedChildLifecycle(t, events, "run_child", "seg_child")
}

func TestSnapshotSubscriptionWaitsForCommittedOpeningPublication(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	executor := &snapshotExecutor{publish: make(chan struct{}), attempted: make(chan struct{})}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	spec := testSegment()
	coordinator.runs = &racingRunProjection{value: runForSegment(spec)}
	committed, release := make(chan struct{}), make(chan struct{})
	spec.CommitOpening = func(ctx context.Context, opening OpeningCommit) error {
		if err := effects.CommitOpening(ctx, opening); err != nil {
			return err
		}
		close(committed)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	opened := make(chan error, 1)
	go func() { _, err := coordinator.openSegment(ctx, spec); opened <- err }()
	<-committed
	attached := make(chan error, 1)
	go func() {
		_, err := coordinator.SubscribeSnapshot(ctx, SubscribeRequest{RunID: spec.RunID, SegmentID: spec.SegmentID}, func(context.Context, string) error { return nil })
		attached <- err
	}()
	var earlyErr error
	early := false
	select {
	case earlyErr = <-attached:
		early = true
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-opened; err != nil {
		t.Fatal(err)
	}
	if !early {
		earlyErr = <-attached
	}
	cancel()
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if earlyErr != nil {
		t.Fatalf("committed opening had no observable owner: %v", earlyErr)
	}
}
