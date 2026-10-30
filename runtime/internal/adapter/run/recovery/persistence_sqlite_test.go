package recovery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	corechat "github.com/Tangerg/scope/core/chat"
)

type alwaysResumable struct{}

func newTestRecovery(
	store runs.RecoveryStore,
	resumability runs.WaitingExecutionResumability,
) (*runs.Recovery, error) {
	return runs.NewRecovery(store, resumability, testsupport.NewAdmissionGate(), nil)
}

func (alwaysResumable) CanResumeWaitingExecution(
	context.Context,
	runs.ExecutionContinuation,
) (runs.WaitingResumption, error) {
	return runs.ResumableWaiting(), nil
}

type goalRunRecorderFunc func(context.Context, run.Run) error

func (g goalRunRecorderFunc) RecordRun(ctx context.Context, value run.Run) error {
	return g(ctx, value)
}

type childRunStartReservationsFunc func(context.Context, string) error

func (c childRunStartReservationsFunc) DeleteSession(ctx context.Context, sessionID string) error {
	return c(ctx, sessionID)
}

func TestRecoveryCleanupIsScopedToClaimedSessions(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Unix(1, 0).UTC()
	draft := run.Draft{
		RunID: "run_abandoned", SessionID: "session_abandoned", SegmentID: "segment_abandoned",
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}
	active, err := run.Admit(draft)
	if err != nil {
		t.Fatalf("Admit domain Run: %v", err)
	}
	runStore := db.Runs
	if err := runStore.Admit(ctx, draft); err != nil {
		t.Fatalf("Admit stored Run: %v", err)
	}
	finishedAt := createdAt.Add(time.Second)
	lost, err := active.RecoverLost(run.Failure{
		Kind: run.FailureLost, Detail: "run lost on restart",
	}, finishedAt, run.MessageMarkAt(0))
	if err != nil {
		t.Fatalf("RecoverLost: %v", err)
	}
	childStarts := db.ChildRunStarts
	if reserveErr := childStarts.Reserve(ctx, persistence.ChildRunStartReservationRecord{
		MemberID: "member_abandoned", SessionID: "session_abandoned",
		Payload: []byte(`{"run":"child_abandoned"}`), CreatedAt: createdAt,
	}); reserveErr != nil {
		t.Fatalf("Reserve: %v", reserveErr)
	}
	checkpointStore := db.ExecutorCheckpoints
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "member_orphan", Payload: []byte(`{"opaque":true}`), BuildID: testsupport.BuildID,
		SessionID: "session_abandoned",
	}
	if saveCheckpointErr := checkpointStore.SaveCheckpoint(ctx, checkpoint); saveCheckpointErr != nil {
		t.Fatalf("SaveCheckpoint: %v", saveCheckpointErr)
	}
	cleanupFailure := errors.New("child-start cleanup failed")
	failingStore, err := New(Config{
		Sessions: db.Sessions, Runs: db.Runs,
		Interrupts: db.Interrupts,
		Transcript: db.Transcript, Messages: mustConversationStore(t, db.ChatHistory),
		GoalRuns:            mustGoalRuns(t, db.Goals),
		ExecutorCheckpoints: checkpointStore,
		ModelInvocations:    db.ModelInvocations,
		ToolInvocations:     db.ToolInvocations,
		ChildRunStarts: childRunStartReservationsFunc(func(context.Context, string) error {
			return cleanupFailure
		}),
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return db.Transactor(ctx, fn)
		},
	})
	if err != nil {
		t.Fatalf("New failing persistence: %v", err)
	}
	commit, err := runs.NewRecoveryCommit(runs.RecoveryCommitInput{
		LostRuns: []run.Replacement{testsupport.MustRunReplacement(active, testsupport.DecidedRun(lost))},
		ConversationTransitions: []runs.RecoveryConversationTransition{{
			RootRunID: active.ID(), SessionID: active.SessionID(), ExpectedCount: 0,
		}},
		DeleteInterrupts:           []string{active.ID()},
		DeleteCheckpointSessionIDs: []string{"session_abandoned"},
	})
	if err != nil {
		t.Fatalf("NewRecoveryCommit: %v", err)
	}
	if commitRecoveryErr := failingStore.CommitRecovery(ctx, commit); !errors.Is(commitRecoveryErr, cleanupFailure) {
		t.Fatalf("failed CommitRecovery = %v, want %v", commitRecoveryErr, cleanupFailure)
	}
	if _, loadCheckpointErr := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); loadCheckpointErr != nil {
		t.Fatalf("cleanup failure did not roll back preceding checkpoint cleanup: %v", loadCheckpointErr)
	}

	store, err := New(Config{
		Sessions: db.Sessions, Runs: db.Runs,
		Interrupts: db.Interrupts,
		Transcript: db.Transcript, Messages: mustConversationStore(t, db.ChatHistory),
		GoalRuns:            mustGoalRuns(t, db.Goals),
		ExecutorCheckpoints: checkpointStore,
		ModelInvocations:    db.ModelInvocations,
		ToolInvocations:     db.ToolInvocations,
		ChildRunStarts:      childStarts,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return db.Transactor(ctx, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	if err := store.CommitRecovery(ctx, commit); err != nil {
		t.Fatalf("CommitRecovery: %v", err)
	}
	var remaining int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM child_run_start_reservations`,
	).Scan(&remaining); err != nil {
		t.Fatalf("count child-start reservations: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("child-start reservations after boot recovery = %d, want 0", remaining)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("orphan checkpoint after successful recovery = %v, want not found", err)
	}
}

type waitingExecutionResumabilityFunc func(context.Context, runs.ExecutionContinuation) (bool, error)

func (w waitingExecutionResumabilityFunc) CanResumeWaitingExecution(
	ctx context.Context,
	continuation runs.ExecutionContinuation,
) (runs.WaitingResumption, error) {
	resumable, err := w(ctx, continuation)
	if err != nil || resumable {
		return runs.ResumableWaiting(), err
	}
	return runs.UnresumableWaiting(runs.LossWaitingStateUnavailable), nil
}

// TestRecoveryRepairsWholeDurableLifecycle proves
// terminal_run_explains_how_it_ended at the runs.recovery boundary.
func TestRecoveryRepairsWholeDurableLifecycle(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	runStore := db.Runs
	sessionStore := db.Sessions
	if insertErr := sessionStore.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{
		ID: "session", Workspace: testsupport.MustWorkspace("/workspace"), CreatedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("seed Session: %v", insertErr)
	}
	interruptStore := db.Interrupts
	transcriptStore := db.Transcript
	messageStore := db.ChatHistory
	goalStore := db.Goals
	checkpointStore := db.ExecutorCheckpoints
	modelInvocations := db.ModelInvocations
	toolInvocations := db.ToolInvocations
	selection, err := modelchoice.New("provider", "model")
	if err != nil {
		t.Fatal(err)
	}
	goalValue, err := goal.New("session", "finish recovery", selection, run.Capabilities{}, "lease_recovery", createdAt)
	if err != nil {
		t.Fatalf("New Goal: %v", err)
	}
	unwritten, err := goal.Unwritten("session")
	if err != nil {
		t.Fatal(err)
	}
	goalReplacement, err := goal.NewReplacement(unwritten.Version(), goalValue)
	if err != nil {
		t.Fatalf("prepare Goal replacement: %v", err)
	}
	if applied, saveErr := goalStore.Save(ctx, goalReplacement); saveErr != nil || !applied {
		t.Fatalf("Save Goal: applied=%t err=%v", applied, saveErr)
	}
	if admitErr := runStore.Admit(ctx, run.Draft{
		RunID: "run_lost", SessionID: "session", SegmentID: "segment", GoalIncarnationID: goalValue.IncarnationID(),
		ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}); admitErr != nil {
		t.Fatalf("Admit: %v", admitErr)
	}
	item := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_running", SessionID: "session", RunID: "run_lost",
		Kind: transcript.QuestionItem, OccurredAt: createdAt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: questioncontract.Text}}},
	})
	if appendItemErr := transcriptStore.AppendItem(ctx, item); appendItemErr != nil {
		t.Fatalf("AppendItem: %v", appendItemErr)
	}
	toolItem := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_tool_running", SessionID: "session", RunID: "run_lost",
		Status: transcript.ItemRunning, Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{Name: "long_running_tool"}, OccurredAt: createdAt.Add(time.Second),
	})
	if appendItemErr := transcriptStore.AppendItem(ctx, toolItem); appendItemErr != nil {
		t.Fatalf("Append Tool Item: %v", appendItemErr)
	}
	modelStartedAt := createdAt.Add(2 * time.Second)
	if startModelInvocationErr := modelInvocations.StartModelInvocation(
		ctx, "session", "run_lost", "segment", "model_call_lost", modelStartedAt, run.ModelConversation); startModelInvocationErr != nil {
		t.Fatalf("start model invocation: %v", startModelInvocationErr)
	}
	toolStartedAt := createdAt.Add(3 * time.Second)
	if startToolInvocationErr := toolInvocations.StartToolInvocation(
		ctx, "session", "run_lost", "segment", "tool_call_lost", toolItem.ID(), toolStartedAt, nil,
	); startToolInvocationErr != nil {
		t.Fatalf("start Tool invocation: %v", startToolInvocationErr)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "orphan_checkpoint",
		Payload:      []byte(`{"opaque":true}`),
		BuildID:      testsupport.BuildID,
		SessionID:    "session",
	}
	if saveCheckpointErr := checkpointStore.SaveCheckpoint(ctx, checkpoint); saveCheckpointErr != nil {
		t.Fatalf("SaveCheckpoint: %v", saveCheckpointErr)
	}
	if _, writeErr := messageStore.Write(
		ctx,
		"session",
		corechat.NewUserMessage(corechat.NewTextPart("recover this")),
		corechat.NewAssistantMessage(corechat.NewToolCallPart(corechat.ToolCall{
			ID: "provider_call_lost", Name: "ask_user", Arguments: "{}",
		})),
	); writeErr != nil {
		t.Fatalf("seed conversation: %v", writeErr)
	}
	rollbackFailure := errors.New("record recovered Goal Run")
	failingPersistence, err := New(Config{
		Sessions:   sessionStore,
		Runs:       runStore,
		Interrupts: interruptStore,
		Transcript: transcriptStore,
		Messages:   mustConversationStore(t, messageStore),
		GoalRuns: goalRunRecorderFunc(func(context.Context, run.Run) error {
			return rollbackFailure
		}),
		ExecutorCheckpoints: checkpointStore,
		ModelInvocations:    modelInvocations,
		ToolInvocations:     toolInvocations,
		ChildRunStarts:      db.ChildRunStarts,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return db.Transactor(ctx, fn)
		},
	})
	if err != nil {
		t.Fatalf("New failing persistence: %v", err)
	}
	failingRecovery, err := newTestRecovery(failingPersistence, alwaysResumable{})
	if err != nil {
		t.Fatalf("New failing Recovery: %v", err)
	}
	if _, reconcileErr := failingRecovery.Reconcile(ctx); !errors.Is(reconcileErr, rollbackFailure) {
		t.Fatalf("failed Reconcile error = %v, want %v", reconcileErr, rollbackFailure)
	}
	afterRollback, err := messageStore.Read(ctx, "session")
	if err != nil || len(afterRollback) != 2 {
		t.Fatalf("conversation after rollback = %#v, %v", afterRollback, err)
	}
	activeAfterRollback, found, err := runStore.Run(ctx, "run_lost")
	if err != nil || !found || activeAfterRollback.State() != run.Running {
		t.Fatalf("Run after rollback = found:%t value:%+v err:%v", found, activeAfterRollback, err)
	}
	if _, loadCheckpointErr := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); loadCheckpointErr != nil {
		t.Fatalf("checkpoint after rollback: %v", loadCheckpointErr)
	}
	var modelState, toolState string
	if scanErr := db.QueryRowContext(ctx,
		`SELECT state FROM model_invocations WHERE call_id = ?`, "model_call_lost",
	).Scan(&modelState); scanErr != nil || modelState != "started" {
		t.Fatalf("model invocation after rollback = %q, %v, want started", modelState, scanErr)
	}
	if scanErr := db.QueryRowContext(ctx,
		`SELECT state FROM tool_invocations WHERE call_id = ? AND segment_id = ?`,
		"tool_call_lost", "segment",
	).Scan(&toolState); scanErr != nil || toolState != "started" {
		t.Fatalf("Tool invocation after rollback = %q, %v, want started", toolState, scanErr)
	}
	persistence, err := New(Config{
		Sessions:            sessionStore,
		Runs:                runStore,
		Interrupts:          interruptStore,
		Transcript:          transcriptStore,
		Messages:            mustConversationStore(t, messageStore),
		GoalRuns:            mustGoalRuns(t, goalStore),
		ExecutorCheckpoints: checkpointStore,
		ModelInvocations:    modelInvocations,
		ToolInvocations:     toolInvocations,
		ChildRunStarts:      db.ChildRunStarts,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return db.Transactor(ctx, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	recovery, err := newTestRecovery(persistence, alwaysResumable{})
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered Runs = %d, want 1", recovered)
	}
	stored, found, err := runStore.Run(ctx, "run_lost")
	failure, failed := stored.Failure()
	if err != nil || !found || stored.State() != run.Failed ||
		!failed || failure.Kind != run.FailureLost || stored.MessageMark() != run.MessageMarkAt(3) {
		t.Fatalf("recovered Run = found:%t value:%+v err:%v", found, stored, err)
	}
	recoveredMessages, err := messageStore.Read(ctx, "session")
	if err != nil || len(recoveredMessages) != 3 || recoveredMessages[2].Role != corechat.RoleTool ||
		len(recoveredMessages[2].Parts) != 1 || recoveredMessages[2].Parts[0].ToolResult == nil ||
		recoveredMessages[2].Parts[0].ToolResult.ID != "provider_call_lost" ||
		!recoveredMessages[2].Parts[0].ToolResult.IsError {
		t.Fatalf("recovered conversation = %#v, %v", recoveredMessages, err)
	}
	storedItem, found, err := transcriptStore.Item(ctx, item.ID())
	if err != nil || !found || storedItem.Status() != transcript.ItemCompleted ||
		!reflect.DeepEqual(storedItem.Snapshot(), item.Snapshot()) {
		t.Fatalf("recovered Item = found:%t value:%+v err:%v", found, storedItem, err)
	}
	recoveredToolItem, found, err := transcriptStore.Item(ctx, toolItem.ID())
	if err != nil || !found || recoveredToolItem.Status() != transcript.ItemIncomplete {
		t.Fatalf("recovered Tool Item = found:%t value:%+v err:%v", found, recoveredToolItem, err)
	}
	if scanErr := db.QueryRowContext(ctx,
		`SELECT state FROM model_invocations WHERE call_id = ?`, "model_call_lost",
	).Scan(&modelState); scanErr != nil || modelState != "unknown" {
		t.Fatalf("recovered model invocation = %q, %v, want retained unknown outcome", modelState, scanErr)
	}
	if scanErr := db.QueryRowContext(ctx,
		`SELECT state FROM tool_invocations WHERE call_id = ? AND segment_id = ?`,
		"tool_call_lost", "segment",
	).Scan(&toolState); scanErr != nil || toolState != "incomplete" {
		t.Fatalf("recovered Tool invocation = %q, %v, want retained incomplete observation", toolState, scanErr)
	}
	if _, loadCheckpointErr := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); !errors.Is(loadCheckpointErr, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("orphan checkpoint after recovery = %v", loadCheckpointErr)
	}
	currentGoal, err := goalStore.Get(ctx, goalValue.SessionID())
	storedGoal, found := currentGoal.Goal()
	if err != nil || !found || storedGoal.Used().Runs != 1 || storedGoal.Status() != goal.StatusPaused || storedGoal.Reason().Code() != goal.ReasonRunNotCompleted {
		t.Fatalf("Goal after recovery = found:%t value:%+v err:%v", found, storedGoal, err)
	}
}

// TestRecoveryRejectsPartialParkWithoutMutatingIt proves
// parked_tree_has_exactly_one_open_interrupt_set at the runs.recovery boundary.
func TestRecoveryRejectsPartialParkWithoutMutatingIt(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	runStore := db.Runs
	sessionStore := db.Sessions
	if insertErr := sessionStore.Insert(ctx, testsupport.MustRestoreSession(session.Snapshot{
		ID: "session", Workspace: testsupport.MustWorkspace("/workspace"), CreatedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("seed Session: %v", insertErr)
	}
	interruptStore := db.Interrupts
	transcriptStore := db.Transcript
	checkpointStore := db.ExecutorCheckpoints
	if admitErr := runStore.Admit(ctx, run.Draft{
		RunID: "run_partial", SessionID: "session", SegmentID: "segment", CreatedAt: createdAt,
		ModelSelection: testsupport.DefaultModelSelection(),
		Capabilities: run.Capabilities{
			InterruptKinds: []run.InterruptKind{run.InterruptQuestion},
		},
	}); admitErr != nil {
		t.Fatalf("Admit: %v", admitErr)
	}
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: questioncontract.Text}}}
	pendingInterrupt := transcript.Interrupt{
		ItemID: "item_missing", ItemOccurredAt: createdAt.Add(time.Second),
		RunID: "run_partial", Kind: run.InterruptQuestion, Question: question,
	}
	if suspendErr := runStore.Suspend(ctx, testsupport.MustRestoreRun(run.Snapshot{ID: "run_partial", SessionID: "session", State: run.Waiting,
		Capabilities: run.Capabilities{
			InterruptKinds: []run.InterruptKind{run.InterruptQuestion},
		},
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(time.Second), MessageMark: run.UnknownMessageMark()}),
		"segment", runs.CommitID{},
	); suspendErr != nil {
		t.Fatalf("Suspend: %v", suspendErr)
	}
	pending := runs.Pending{
		RootRunID: "run_partial", ExecutorID: "turn_partial",
		Interrupts: runs.OpenInterruptsOf([]transcript.Interrupt{pendingInterrupt}),
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: pendingInterrupt.ItemID, MemberID: "member_root", RequestID: "request_root",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_partial", MemberID: "member_root",
		}},
		CreatedAt: createdAt.Add(time.Second)}
	if openErr := interruptStore.Open(ctx, pending); openErr != nil {
		t.Fatalf("Open Pending: %v", openErr)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "member_root", Payload: []byte(`{"opaque":true}`), BuildID: testsupport.BuildID,
		SessionID: "session",
	}
	if saveCheckpointErr := checkpointStore.SaveCheckpoint(ctx, checkpoint); saveCheckpointErr != nil {
		t.Fatalf("SaveCheckpoint: %v", saveCheckpointErr)
	}
	persistence, err := New(Config{
		Sessions: sessionStore, Runs: runStore, Interrupts: interruptStore, Transcript: transcriptStore,
		Messages: mustConversationStore(t, db.ChatHistory), GoalRuns: mustGoalRuns(t, db.Goals), ExecutorCheckpoints: checkpointStore,
		ModelInvocations: db.ModelInvocations, ToolInvocations: db.ToolInvocations,
		ChildRunStarts: db.ChildRunStarts,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return db.Transactor(ctx, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	recovery, err := newTestRecovery(persistence, alwaysResumable{})
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, reconcileErr := recovery.Reconcile(ctx); reconcileErr == nil {
		t.Fatal("Reconcile accepted a Pending whose interrupt Item is absent")
	}
	if _, found, getErr := interruptStore.Get(ctx, pending.RootRunID); getErr != nil || !found {
		t.Fatalf("Pending after rejection = found:%t err:%v, want preserved", found, getErr)
	}
	stored, found, err := runStore.Run(ctx, pending.RootRunID)
	if err != nil || !found || stored.State() != run.Waiting {
		t.Fatalf("Run after rejection = found:%t value:%+v err:%v", found, stored, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); err != nil {
		t.Fatalf("checkpoint after rejection: %v", err)
	}
}

// mustConversationStore wraps the durable history store in the Session-keyed
// port recovery addresses it by.
func mustConversationStore(t *testing.T, messages *persistence.MessageStore) *persistence.ConversationStore {
	t.Helper()
	store, err := persistence.NewConversationStore(messages)
	if err != nil {
		t.Fatalf("conversation store: %v", err)
	}
	return store
}

func mustGoalRuns(tb testing.TB, store goals.Store) *goals.RunRecorder {
	tb.Helper()
	recorder, err := goals.NewRunRecorder(store)
	if err != nil {
		tb.Fatal(err)
	}
	return recorder
}
