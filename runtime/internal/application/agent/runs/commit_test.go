package runs

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	corechat "github.com/Tangerg/scope/core/chat"
)

func TestEventCommitDerivesLifecycleFromRun(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	waiting := testsupport.MustRestoreRun(run.Snapshot{ID: "run_1", SessionID: "session", State: run.Waiting,
		CreatedAt: createdAt, UpdatedAt: createdAt, MessageMark: run.UnknownMessageMark})
	config := EventCommitConfig{RunID: waiting.ID(), SessionID: waiting.SessionID(), SegmentID: "segment_1", Run: &waiting}
	commit := mustEventCommit(t, config)
	if !commit.Suspends() || commit.Terminates() || !commit.ChangesLifecycle() || commit.GoalRun() != nil {
		t.Fatalf("waiting lifecycle = %+v", commit)
	}
	config.Run = nil
	ordinary := mustEventCommit(t, config)
	if ordinary.Suspends() || ordinary.Terminates() || ordinary.ChangesLifecycle() {
		t.Fatal("ordinary commit changed lifecycle")
	}
	value := testsupport.MustRestoreRun(run.Snapshot{ID: "run_1", SessionID: "session", State: run.Running})
	config.Run = &value
	if _, err := NewEventCommit(config); err == nil {
		t.Fatal("accepted an execution Run as a lifecycle transition")
	}

	zero := run.Run{}
	config.Run = &zero
	if _, err := NewEventCommit(config); err == nil {
		t.Fatal("accepted zero Run")
	}
	config.Run = &waiting
	config.RunID = "run_other"
	if _, err := NewEventCommit(config); err == nil {
		t.Fatal("accepted foreign Run")
	}
}

func TestTerminalEventCommitAllowsOnlyTheTransactionalWatermarkPlaceholder(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	outcome := run.OutcomeCanceled
	record := testsupport.MustRestoreRun(run.Snapshot{ID: "run_1", SessionID: "session", State: run.Canceled,
		Outcome: &outcome, CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Second),
		FinishedAt: createdAt.Add(time.Second), MessageMark: run.UnknownMessageMark})

	config := EventCommitConfig{
		RunID: record.ID(), SessionID: record.SessionID(), SegmentID: "segment_1",
		CommitID: testCommitID("run_commit_event_1"), Run: &record,
	}
	if _, err := NewEventCommit(config); err != nil {
		t.Fatalf("terminal commit awaiting transactional watermark: %v", err)
	}
	config.CommitID = runtimeidentity.CommitID{}
	if _, err := NewEventCommit(config); err == nil {
		t.Fatal("terminal commit without an immutable commit identity passed validation")
	}

	invalid := record.Snapshot()
	invalid.MessageMark = run.UnknownMessageMark - 1
	if _, err := run.Restore(invalid); err == nil {
		t.Fatal("Run.Restore accepted an invalid negative message watermark")
	}
}

func TestEventCommitToolJournalOwnsMatchingItemState(t *testing.T) {
	startedAt := time.Date(2026, 8, 13, 2, 3, 4, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	running := testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "session", RunID: "run_1", ID: "item_1",
		Status: transcript.ItemRunning, Kind: transcript.ToolCall, OccurredAt: startedAt,
	})
	completed := testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "session", RunID: "run_1", ID: "item_1",
		Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
		OccurredAt: startedAt, FinishedAt: finishedAt,
	})
	failed := testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "session", RunID: "run_1", ID: "item_1",
		Status: transcript.ItemIncomplete, Kind: transcript.ToolCall,
		OccurredAt: startedAt, FinishedAt: finishedAt,
		Failure: &tool.Failure{Kind: tool.FailureExecution},
	})
	unknown := testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "session", RunID: "run_1", ID: "item_1",
		Status: transcript.ItemIncomplete, Kind: transcript.ToolCall,
		OccurredAt: startedAt, FinishedAt: finishedAt,
	})
	started := ToolInvocationCommit{
		CallID: "call_1", ItemID: running.ID(), SegmentID: "segment_1",
		State: ToolInvocationStarted, StartedAt: startedAt,
	}
	terminal := started
	terminal.State = ToolInvocationCompleted
	terminal.FinishedAt = finishedAt

	tests := []struct {
		name       string
		items      []transcript.Item
		invocation ToolInvocationCommit
		wantErr    bool
	}{
		{name: "missing Item", invocation: started, wantErr: true},
		{name: "started with terminal Item", items: []transcript.Item{completed}, invocation: started, wantErr: true},
		{name: "completed with running Item", items: []transcript.Item{running}, invocation: terminal, wantErr: true},
		{name: "completed with unclassified Item", items: []transcript.Item{unknown}, invocation: terminal, wantErr: true},
		{name: "matched start", items: []transcript.Item{running}, invocation: started},
		{name: "matched completion", items: []transcript.Item{completed}, invocation: terminal},
		{name: "matched failed completion", items: []transcript.Item{failed}, invocation: terminal},
		{name: "different Segment", items: []transcript.Item{running}, invocation: ToolInvocationCommit{
			CallID: "call_1", ItemID: running.ID(), SegmentID: "segment_2",
			State: ToolInvocationStarted, StartedAt: startedAt,
		}, wantErr: true},
		{name: "parked attempt", items: []transcript.Item{running}, invocation: ToolInvocationCommit{
			CallID: "call_1", ItemID: running.ID(), SegmentID: "segment_1",
			State: ToolInvocationIncomplete, StartedAt: startedAt, FinishedAt: finishedAt,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewEventCommit(EventCommitConfig{
				RunID: "run_1", SessionID: "session", SegmentID: "segment_1", Items: test.items,
				ToolInvocations: []ToolInvocationCommit{test.invocation},
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestEventCommitOwnsInvocationAndProgressSegment(t *testing.T) {
	startedAt := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	config := EventCommitConfig{
		RunID: "run_1", SessionID: "session", SegmentID: "segment_1",
		ModelInvocations: []ModelInvocationCommit{{
			CallID: "call_1", SegmentID: "segment_2",
			State: ModelInvocationStarted, StartedAt: startedAt,
		}},
	}
	if _, err := NewEventCommit(config); err == nil {
		t.Fatal("EventCommit accepted a model invocation from another Segment")
	}
	config.ModelInvocations = nil
	config.Progress = &ProgressCommit{
		SegmentID: "segment_2", Metrics: run.Metrics{}, UpdatedAt: startedAt,
	}
	if _, err := NewEventCommit(config); err == nil {
		t.Fatal("EventCommit accepted progress from another Segment")
	}
}

func TestOpeningCommitValidatesItsLifecycleAction(t *testing.T) {
	createdAt := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	invalidAdmission := run.Draft{
		RunID: "run_1", SessionID: "session", ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}
	invalidResume := run.TreeResumeDraft{
		RootRunID: "run_1", SessionID: "session", ResumedAt: createdAt,
	}
	for _, test := range []struct {
		name  string
		build func() error
	}{
		{name: "admission", build: func() error {
			_, err := NewAdmissionOpeningCommit(
				testCommitID("run_commit_invalid_admission"), invalidAdmission,
				nil, nil, "", nil, nil,
			)
			return err
		}},
		{name: "resume", build: func() error {
			_, err := NewResumeOpeningCommit(
				testCommitID("run_commit_invalid_resume"), invalidResume, nil,
			)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.build(); err == nil {
				t.Fatalf("OpeningCommit accepted an invalid %s action", test.name)
			}
		})
	}
}

func TestOpeningCommitRejectsRootFactsOutsideARootAdmission(t *testing.T) {
	createdAt := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	initialSession := testsupport.MustRestoreSession(session.Snapshot{
		ID: "session", Workspace: testsupport.MustWorkspace("/work"),
		StartedAt: createdAt, UpdatedAt: createdAt, Revision: 1,
	})
	child := run.Draft{
		RunID: "run_child", SessionID: initialSession.ID(), SegmentID: "segment_child",
		SpawnedByItemID: "item_spawn", ParentRunID: "run_root", RootRunID: "run_root",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}
	if _, err := NewAdmissionOpeningCommit(
		testCommitID("run_commit_child_with_session"), child,
		&initialSession, nil, "", nil, nil,
	); err == nil {
		t.Fatal("OpeningCommit accepted root Session facts on a child admission")
	}

	root := run.Draft{
		RunID: "run_scheduled", SessionID: "session_scheduled", SegmentID: "segment_scheduled",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}
	if _, err := NewAdmissionOpeningCommit(
		testCommitID("run_commit_schedule_without_session"), root,
		nil, nil, "sch_test:1000", nil, nil,
	); err == nil {
		t.Fatal("OpeningCommit accepted a schedule admission without its initial Session")
	}
	if _, err := NewAdmissionOpeningCommit(
		testCommitID("run_commit_existing_session"), root,
		nil, nil, "", nil, nil,
	); err != nil {
		t.Fatalf("ordinary admission into an existing Session: %v", err)
	}
}

func TestOpeningCommitOwnsEveryOpeningEvent(t *testing.T) {
	createdAt := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	item := func(runID, itemID string) transcript.Item {
		return testsupport.MustRestoreItem(testsupport.ItemInput{
			SessionID: "session", RunID: runID, ID: itemID, OccurredAt: createdAt,
		})
	}
	root := run.Draft{
		RunID: "run_root", SessionID: "session", SegmentID: "segment_root",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}
	foreign := mustEventCommit(t, EventCommitConfig{
		RunID: "run_foreign", SessionID: "session", SegmentID: "segment_foreign",
		Items: []transcript.Item{item("run_foreign", "item_foreign")},
	})
	if _, err := NewAdmissionOpeningCommit(
		testCommitID("run_commit_foreign_event"), root,
		nil, nil, "", nil, []EventCommit{foreign},
	); err == nil {
		t.Fatal("root OpeningCommit accepted an event for another Run")
	}

	child := run.Draft{
		RunID: "run_child", SessionID: "session", SegmentID: "segment_child",
		SpawnedByItemID: "item_spawn", ParentRunID: root.RunID, RootRunID: root.RunID,
		ModelSelection: root.ModelSelection, CreatedAt: createdAt,
	}
	parentEvent := mustEventCommit(t, EventCommitConfig{
		RunID: root.RunID, SessionID: "session", SegmentID: root.SegmentID,
		Items: []transcript.Item{item(root.RunID, "item_parent")},
	})
	childEvent := mustEventCommit(t, EventCommitConfig{
		RunID: child.RunID, SessionID: "session", SegmentID: child.SegmentID,
		Items: []transcript.Item{item(child.RunID, "item_child")},
	})
	if _, err := NewAdmissionOpeningCommit(
		testCommitID("run_commit_child_events"), child,
		nil, nil, "", nil, []EventCommit{parentEvent, childEvent},
	); err != nil {
		t.Fatalf("child OpeningCommit rejected its parent/child projections: %v", err)
	}
	withProgress := mustEventCommit(t, EventCommitConfig{
		RunID: child.RunID, SessionID: child.SessionID, SegmentID: child.SegmentID, Items: childEvent.Items(),
		Progress: &ProgressCommit{SegmentID: child.SegmentID, UpdatedAt: createdAt, Metrics: run.Metrics{}},
	})
	if _, err := NewAdmissionOpeningCommit(
		testCommitID("run_commit_child_progress"), child,
		nil, nil, "", nil, []EventCommit{parentEvent, withProgress},
	); err == nil {
		t.Fatal("child OpeningCommit accepted an execution observation")
	}

	resume := run.TreeResumeDraft{
		RootRunID: root.RunID, SessionID: "session", ResumedAt: createdAt,
		Runs: []run.ResumeDraft{{RunID: root.RunID, SegmentID: "segment_resumed"}},
	}
	wrongSegment := mustEventCommit(t, EventCommitConfig{
		RunID: root.RunID, SessionID: "session", SegmentID: "segment_stale",
		Items: []transcript.Item{item(root.RunID, "item_resumed")},
	})
	if _, err := NewResumeOpeningCommit(
		testCommitID("run_commit_stale_resume_event"), resume, []EventCommit{wrongSegment},
	); err == nil {
		t.Fatal("resumed OpeningCommit accepted an event for a stale Segment")
	}
}

func TestCompositeCommitsRejectNestedTopLevelEventIdentity(t *testing.T) {
	createdAt := time.Date(2026, 8, 15, 2, 3, 4, 0, time.UTC)
	openingItem := testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: "session", RunID: "run_root", ID: "item_opening", OccurredAt: createdAt,
	})
	admission := run.Draft{
		RunID: "run_root", SessionID: "session", SegmentID: "segment_root",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}
	if _, err := NewAdmissionOpeningCommit(
		testCommitID("run_commit_opening_parent"), admission,
		nil, nil, "", nil, []EventCommit{mustEventCommit(t, EventCommitConfig{
			RunID: admission.RunID, SessionID: admission.SessionID, SegmentID: admission.SegmentID,
			CommitID: testCommitID("run_commit_opening_nested"), Items: []transcript.Item{openingItem},
		})},
	); err == nil {
		t.Fatal("OpeningCommit accepted a nested top-level event identity")
	}

	pending := testApprovalPending("member_root", createdAt)
	waiting := runForPending(pending)
	checkpoint := testExecutorCheckpoint()
	root, _ := pending.RootContinuation()
	checkpointState := checkpoint.State()
	checkpointState.ModelSelection = root.ModelSelection
	checkpointState.Limits = root.Limits
	checkpointState.Capabilities = pending.Capabilities
	checkpoint = testsupport.MustCheckpoint(checkpointState)
	_, err := NewTreeBarrierCommit(
		testCommitID("run_commit_barrier_parent"),
		pending,
		[]EventCommit{mustEventCommit(t, EventCommitConfig{
			RunID: waiting.ID(), SessionID: waiting.SessionID(), SegmentID: "segment_root",
			CommitID: testCommitID("run_commit_barrier_nested"), Run: &waiting,
		})},
		checkpoint,
	)
	if err == nil {
		t.Fatal("TreeBarrierCommit accepted a nested top-level event identity")
	}
}

func TestTreeBarrierCommitOwnsItsValidatedWriteSet(t *testing.T) {
	createdAt := time.Date(2026, 9, 5, 1, 2, 3, 0, time.UTC)
	pending := testApprovalPending("member_root", createdAt)
	waiting := runForPending(pending)
	checkpoint := testExecutorCheckpoint()
	root, _ := pending.RootContinuation()
	checkpointState := checkpoint.State()
	checkpointState.ModelSelection = root.ModelSelection
	checkpointState.Limits = root.Limits
	checkpointState.Capabilities = pending.Capabilities
	checkpoint = testsupport.MustCheckpoint(checkpointState)
	commits := []EventCommit{mustEventCommit(t, EventCommitConfig{
		RunID: waiting.ID(), SessionID: waiting.SessionID(), SegmentID: "segment_root",
		Run: &waiting,
		ConversationMessages: []corechat.Message{
			corechat.NewUserMessage(corechat.NewTextPart("original")),
		},
	})}

	barrier, err := NewTreeBarrierCommit(
		testCommitID("run_commit_owned_barrier"), pending, commits, checkpoint,
	)
	if err != nil {
		t.Fatalf("NewTreeBarrierCommit: %v", err)
	}

	pending.Bindings[0].MemberID = "member_changed"
	*commits[0].Run() = run.Run{}
	commits[0].ConversationMessages()[0].Parts[0].Text = "changed"
	checkpoint.Payload()[0] = 'x'

	projectedPending := barrier.Pending()
	projectedPending.Bindings[0].MemberID = "member_projected"
	projectedRuns := barrier.Runs()
	*projectedRuns[0].Run() = run.Run{}
	projectedRuns[0].ConversationMessages()[0].Parts[0].Text = "projected"
	projectedCheckpoint := barrier.Checkpoint()
	projectedCheckpoint.Payload()[0] = 'y'

	ownedPending := barrier.Pending()
	ownedRuns := barrier.Runs()
	ownedCheckpoint := barrier.Checkpoint()
	if ownedPending.Bindings[0].MemberID != "member_root" {
		t.Fatalf("owned Pending member = %q, want member_root", ownedPending.Bindings[0].MemberID)
	}
	if ownedRuns[0].Run() == nil || ownedRuns[0].ConversationMessages()[0].Text() != "original" {
		t.Fatalf("owned Run commit = %+v, want isolated Run and message", ownedRuns[0])
	}
	if string(ownedCheckpoint.Payload()) != `{"root":"member_root"}` {
		t.Fatalf("owned checkpoint payload = %q", ownedCheckpoint.Payload())
	}
	if err := barrier.Validate(); err != nil {
		t.Fatalf("owned barrier no longer validates: %v", err)
	}
}

func TestOpeningCommitOwnsItsValidatedWriteSet(t *testing.T) {
	createdAt := time.Date(2026, 9, 5, 2, 3, 4, 0, time.UTC)
	admission := run.Draft{
		RunID: "run_root", SessionID: "session", SegmentID: "segment_root",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
		Capabilities: run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
	}
	initialSession := testsupport.MustRestoreSession(session.Snapshot{
		ID: "session", Workspace: testsupport.MustWorkspace("/work"),
		StartedAt: createdAt, UpdatedAt: createdAt, Revision: 1,
	})
	events := []EventCommit{mustEventCommit(t, EventCommitConfig{
		RunID: admission.RunID, SessionID: admission.SessionID, SegmentID: admission.SegmentID,
		ConversationMessages: []corechat.Message{
			corechat.NewUserMessage(corechat.NewTextPart("original")),
		},
	})}

	opening, err := NewAdmissionOpeningCommit(
		testCommitID("run_commit_owned_opening"), admission,
		&initialSession, nil, "", nil, events,
	)
	if err != nil {
		t.Fatalf("NewAdmissionOpeningCommit: %v", err)
	}

	admission.Capabilities.InterruptKinds[0] = interrupt.Question
	initialSession = session.Session{}
	events[0].ConversationMessages()[0].Parts[0].Text = "changed"
	projectedAdmission, _ := opening.Admission()
	projectedAdmission.Capabilities.InterruptKinds[0] = interrupt.Question
	projectedEvents := opening.Events()
	projectedEvents[0].ConversationMessages()[0].Parts[0].Text = "projected"

	ownedAdmission, admitted := opening.Admission()
	ownedSession, initialized := opening.InitialSession()
	ownedEvents := opening.Events()
	if !admitted || ownedAdmission.Capabilities.InterruptKinds[0] != interrupt.Approval {
		t.Fatalf("owned admission = %+v", ownedAdmission)
	}
	if !initialized || ownedSession.ID() != "session" {
		t.Fatalf("owned initial Session = %+v", ownedSession)
	}
	if ownedEvents[0].ConversationMessages()[0].Text() != "original" {
		t.Fatalf("owned opening events = %+v", ownedEvents)
	}
	if err := opening.Validate(); err != nil {
		t.Fatalf("owned admission opening no longer validates: %v", err)
	}

	resume := run.TreeResumeDraft{
		RootRunID: "run_root", SessionID: "session", ResumedAt: createdAt,
		Runs: []run.ResumeDraft{{RunID: "run_root", SegmentID: "segment_resumed"}},
	}
	resumed, err := NewResumeOpeningCommit(
		testCommitID("run_commit_owned_resume"), resume, nil,
	)
	if err != nil {
		t.Fatalf("NewResumeOpeningCommit: %v", err)
	}
	resume.Runs[0].SegmentID = "segment_changed"
	projectedResume, _ := resumed.Resume()
	projectedResume.Runs[0].SegmentID = "segment_projected"
	ownedResume, ok := resumed.Resume()
	if !ok || ownedResume.Runs[0].SegmentID != "segment_resumed" {
		t.Fatalf("owned resume = %+v", ownedResume)
	}
	if err := resumed.Validate(); err != nil {
		t.Fatalf("owned resume opening no longer validates: %v", err)
	}
}

func mustEventCommit(t *testing.T, config EventCommitConfig) EventCommit {
	t.Helper()
	commit, err := NewEventCommit(config)
	if err != nil {
		t.Fatalf("NewEventCommit: %v", err)
	}
	return commit
}

func TestEventCommitOwnsItsWriteSetAndDerivesGoalAccounting(t *testing.T) {
	startedAt := time.Date(2026, 9, 7, 1, 2, 3, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	outcome := run.OutcomeCompleted
	record := testsupport.MustRestoreRun(run.Snapshot{
		ID: "run_1", SessionID: "session", State: run.Completed, Outcome: &outcome,
		GoalIncarnationID: "goal_1", CreatedAt: startedAt, UpdatedAt: finishedAt, FinishedAt: finishedAt,
		Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{Steps: 2}),
	})
	item := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_1", RunID: record.ID(), SessionID: record.SessionID(), Kind: transcript.ToolCall,
		Status: transcript.ItemCompleted, OccurredAt: startedAt, FinishedAt: finishedAt,
	})
	config := EventCommitConfig{
		RunID: record.ID(), SessionID: record.SessionID(), SegmentID: "segment_1",
		CommitID: testCommitID("run_commit_owned_event"), Run: &record,
		Items:                []transcript.Item{item},
		ConversationMessages: []corechat.Message{corechat.NewAssistantMessage(corechat.NewTextPart("original"))},
		ModelInvocations:     []ModelInvocationCommit{{CallID: "model_1", SegmentID: "segment_1", State: ModelInvocationCompleted, StartedAt: startedAt, FinishedAt: finishedAt}},
		ToolInvocations:      []ToolInvocationCommit{{CallID: "tool_1", ItemID: item.ID(), SegmentID: "segment_1", State: ToolInvocationCompleted, StartedAt: startedAt, FinishedAt: finishedAt}},
		Progress:             &ProgressCommit{SegmentID: "segment_1", Metrics: record.Metrics(), UpdatedAt: finishedAt},
	}
	commit := mustEventCommit(t, config)
	config.Items[0] = transcript.Item{}
	config.ConversationMessages[0].Parts[0].Text = "changed"
	config.ModelInvocations[0].CallID = "changed"
	config.ToolInvocations[0].ItemID = "changed"
	config.Progress.SegmentID = "changed"
	record = run.Run{}
	commit.Items()[0] = transcript.Item{}
	commit.ConversationMessages()[0].Parts[0].Text = "projected"
	commit.ModelInvocations()[0].CallID = "projected"
	commit.ToolInvocations()[0].ItemID = "projected"
	commit.Progress().SegmentID = "projected"
	*commit.Run() = run.Run{}
	commit.GoalRun().RunID = "projected"
	if commit.Items()[0].ID() != item.ID() || commit.ConversationMessages()[0].Text() != "original" ||
		commit.ModelInvocations()[0].CallID != "model_1" || commit.ToolInvocations()[0].ItemID != item.ID() ||
		commit.Progress().SegmentID != "segment_1" || commit.Run().ID() != "run_1" {
		t.Fatal("event commit followed input or output mutation")
	}
	charge := commit.GoalRun()
	if !commit.Terminates() || commit.Suspends() || !commit.ChangesLifecycle() || charge == nil ||
		charge.RunID != "run_1" || charge.SessionID != "session" || charge.IncarnationID != "goal_1" ||
		charge.Outcome != outcome || charge.Steps != 2 || !charge.CompletedAt.Equal(finishedAt) {
		t.Fatalf("terminal Goal accounting = %+v", charge)
	}
}

func TestEventCommitRetiresCheckpointsOnlyForTerminalRoots(t *testing.T) {
	now := time.Date(2026, 9, 7, 1, 2, 3, 0, time.UTC)
	terminal := testsupport.MustRestoreRun(run.Snapshot{ID: "run_root", SessionID: "session", State: run.Completed, CreatedAt: now, FinishedAt: now})
	waiting := testsupport.MustRestoreRun(run.Snapshot{ID: "run_root", SessionID: "session", State: run.Waiting, CreatedAt: now})
	child := testsupport.MustRestoreRun(run.Snapshot{ID: "run_child", SessionID: "session", State: run.Completed, CreatedAt: now, FinishedAt: now,
		Lineage: run.Lineage{ParentRunID: "run_root", RootRunID: "run_root", SpawnedByItemID: "item_spawn"},
	})
	for _, test := range []struct {
		name   string
		record *run.Run
		valid  bool
	}{
		{name: "ordinary"}, {name: "waiting", record: &waiting}, {name: "terminal child", record: &child}, {name: "terminal root", record: &terminal, valid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := EventCommitConfig{RunID: "run_root", SessionID: "session", SegmentID: "segment_1", CommitID: testCommitID("run_commit_retire_checkpoint"), Run: test.record, ObsoleteCheckpointRootID: "member_root"}
			if test.record != nil {
				config.RunID = test.record.ID()
			}
			_, err := NewEventCommit(config)
			if (err == nil) != test.valid {
				t.Fatalf("NewEventCommit error = %v, want valid %t", err, test.valid)
			}
		})
	}
}
