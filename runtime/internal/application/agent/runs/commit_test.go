package runs

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestEventCommitUsesCompleteRunStateInvariant(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	waiting := testsupport.MustRestoreRun(run.Snapshot{ID: "run_1", SessionID: "session", State: run.Waiting,
		CreatedAt: createdAt, UpdatedAt: createdAt,
		MessageMark: run.UnknownMessageMark})

	valid := EventCommit{
		RunID: waiting.ID(), SessionID: waiting.SessionID(), SegmentID: "segment_1",
		State: StateSuspend, Run: &waiting,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid suspend commit: %v", err)
	}
	withOutcome := valid
	withOutcome.Outcome = run.OutcomeCanceled
	if err := withOutcome.Validate(); err == nil {
		t.Fatal("suspend commit accepted a terminal outcome")
	}
	unchangedWithOutcome := EventCommit{
		RunID: "run_1", SessionID: "session", SegmentID: "segment_1",
		Outcome: run.OutcomeCanceled,
	}
	if err := unchangedWithOutcome.Validate(); err == nil || unchangedWithOutcome.isEmpty() {
		t.Fatalf("unchanged commit with outcome = empty:%t error:%v", unchangedWithOutcome.isEmpty(), err)
	}

	contradictory := waiting.Snapshot()
	contradictory.ActiveSegmentID = "segment_stale"
	if _, err := run.Restore(contradictory); err == nil {
		t.Fatal("Run.Restore accepted a waiting Run with an active Segment")
	}
}

func TestTerminalEventCommitAllowsOnlyTheTransactionalWatermarkPlaceholder(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	outcome := run.OutcomeCanceled
	record := testsupport.MustRestoreRun(run.Snapshot{ID: "run_1", SessionID: "session", State: run.Canceled,
		Outcome: &outcome, CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Second),
		FinishedAt: createdAt.Add(time.Second), MessageMark: run.UnknownMessageMark})

	commit := EventCommit{
		RunID: record.ID(), SessionID: record.SessionID(), SegmentID: "segment_1", State: StateTerminalize,
		CommitID: testCommitID("run_commit_event_1"), Outcome: outcome, Run: &record,
	}
	if err := commit.Validate(); err != nil {
		t.Fatalf("terminal commit awaiting transactional watermark: %v", err)
	}
	commit.CommitID = runtimeidentity.CommitID{}
	if err := commit.Validate(); err == nil {
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
			err := (EventCommit{
				RunID: "run_1", SessionID: "session", SegmentID: "segment_1", Items: test.items,
				ToolInvocations: []ToolInvocationCommit{test.invocation},
			}).Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestEventCommitOwnsInvocationAndProgressSegment(t *testing.T) {
	startedAt := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	commit := EventCommit{
		RunID: "run_1", SessionID: "session", SegmentID: "segment_1",
		ModelInvocations: []ModelInvocationCommit{{
			CallID: "call_1", SegmentID: "segment_2",
			State: ModelInvocationStarted, StartedAt: startedAt,
		}},
	}
	if err := commit.Validate(); err == nil {
		t.Fatal("EventCommit accepted a model invocation from another Segment")
	}
	commit.ModelInvocations = nil
	commit.Progress = &ProgressCommit{
		SegmentID: "segment_2", Metrics: run.Metrics{}, UpdatedAt: startedAt,
	}
	if err := commit.Validate(); err == nil {
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
		name    string
		opening OpeningCommit
	}{
		{name: "admission", opening: OpeningCommit{
			CommitID: testCommitID("run_commit_invalid_admission"), Admit: &invalidAdmission,
		}},
		{name: "resume", opening: OpeningCommit{
			CommitID: testCommitID("run_commit_invalid_resume"), Resume: &invalidResume,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.opening.Validate(); err == nil {
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
	if err := (OpeningCommit{
		CommitID: testCommitID("run_commit_child_with_session"), Admit: &child, InitialSession: &initialSession,
	}).Validate(); err == nil {
		t.Fatal("OpeningCommit accepted root Session facts on a child admission")
	}

	root := run.Draft{
		RunID: "run_scheduled", SessionID: "session_scheduled", SegmentID: "segment_scheduled",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}
	if err := (OpeningCommit{
		CommitID: testCommitID("run_commit_schedule_without_session"), Admit: &root, ScheduleFiring: "sch_test:1000",
	}).Validate(); err == nil {
		t.Fatal("OpeningCommit accepted a schedule admission without its initial Session")
	}
	if err := (OpeningCommit{
		CommitID: testCommitID("run_commit_existing_session"), Admit: &root,
	}).Validate(); err != nil {
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
	foreign := EventCommit{
		RunID: "run_foreign", SessionID: "session", SegmentID: "segment_foreign",
		Items: []transcript.Item{item("run_foreign", "item_foreign")},
	}
	if err := (OpeningCommit{
		CommitID: testCommitID("run_commit_foreign_event"), Admit: &root, Events: []EventCommit{foreign},
	}).Validate(); err == nil {
		t.Fatal("root OpeningCommit accepted an event for another Run")
	}

	child := run.Draft{
		RunID: "run_child", SessionID: "session", SegmentID: "segment_child",
		SpawnedByItemID: "item_spawn", ParentRunID: root.RunID, RootRunID: root.RunID,
		ModelSelection: root.ModelSelection, CreatedAt: createdAt,
	}
	parentEvent := EventCommit{
		RunID: root.RunID, SessionID: "session", SegmentID: root.SegmentID,
		Items: []transcript.Item{item(root.RunID, "item_parent")},
	}
	childEvent := EventCommit{
		RunID: child.RunID, SessionID: "session", SegmentID: child.SegmentID,
		Items: []transcript.Item{item(child.RunID, "item_child")},
	}
	if err := (OpeningCommit{
		CommitID: testCommitID("run_commit_child_events"), Admit: &child,
		Events: []EventCommit{parentEvent, childEvent},
	}).Validate(); err != nil {
		t.Fatalf("child OpeningCommit rejected its parent/child projections: %v", err)
	}
	withProgress := childEvent
	withProgress.Progress = &ProgressCommit{
		SegmentID: child.SegmentID, UpdatedAt: createdAt, Metrics: run.Metrics{},
	}
	if err := (OpeningCommit{
		CommitID: testCommitID("run_commit_child_progress"), Admit: &child,
		Events: []EventCommit{parentEvent, withProgress},
	}).Validate(); err == nil {
		t.Fatal("child OpeningCommit accepted an execution observation")
	}

	resume := run.TreeResumeDraft{
		RootRunID: root.RunID, SessionID: "session", ResumedAt: createdAt,
		Runs: []run.ResumeDraft{{RunID: root.RunID, SegmentID: "segment_resumed"}},
	}
	wrongSegment := EventCommit{
		RunID: root.RunID, SessionID: "session", SegmentID: "segment_stale",
		Items: []transcript.Item{item(root.RunID, "item_resumed")},
	}
	if err := (OpeningCommit{
		CommitID: testCommitID("run_commit_stale_resume_event"), Resume: &resume,
		Events: []EventCommit{wrongSegment},
	}).Validate(); err == nil {
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
	if err := (OpeningCommit{
		CommitID: testCommitID("run_commit_opening_parent"), Admit: &admission,
		Events: []EventCommit{{
			RunID: admission.RunID, SessionID: admission.SessionID, SegmentID: admission.SegmentID,
			CommitID: testCommitID("run_commit_opening_nested"), Items: []transcript.Item{openingItem},
		}},
	}).Validate(); err == nil {
		t.Fatal("OpeningCommit accepted a nested top-level event identity")
	}

	pending := testApprovalPending("member_root", createdAt)
	waiting := runForPending(pending)
	checkpoint := testExecutorCheckpoint()
	root, _ := pending.RootContinuation()
	checkpoint.ModelSelection = root.ModelSelection
	checkpoint.Limits = root.Limits
	checkpoint.Capabilities = pending.Capabilities
	barrier := TreeBarrierCommit{
		CommitID: testCommitID("run_commit_barrier_parent"), Pending: pending, Checkpoint: checkpoint,
		Runs: []EventCommit{{
			RunID: waiting.ID(), SessionID: waiting.SessionID(), SegmentID: "segment_root",
			CommitID: testCommitID("run_commit_barrier_nested"), State: StateSuspend, Run: &waiting,
		}},
	}
	if err := barrier.Validate(); err == nil {
		t.Fatal("TreeBarrierCommit accepted a nested top-level event identity")
	}
}
