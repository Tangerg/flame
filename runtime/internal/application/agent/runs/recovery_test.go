package runs

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	interruptdomain "github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	corechat "github.com/Tangerg/scope/core/chat"
)

func newTestRecovery(
	store RecoveryStore,
	resumability WaitingExecutionResumability,
) (*Recovery, error) {
	return NewRecovery(store, resumability, testsupport.NewAdmissionGate(), nil)
}

type recoveryStoreStub struct {
	runs         []rundomain.Run
	pending      []Pending
	models       []OpenModelInvocation
	tools        []OpenToolInvocation
	transcripts  map[string][]transcript.Item
	messageMarks map[string]int
	messages     map[string][]corechat.Message
	sessions     map[string]session.Session

	commit            RecoveryCommit
	commits           int
	commitErr         error
	checkpoint        *ExecutorCheckpoint
	checkpointErr     error
	aliasOpen         bool
	aliasPending      bool
	transcriptReads   int
	conversationReads int
	mutateCommitViews bool
}

func invalidRecoveryCommit(
	commit RecoveryCommit,
	mutate func(*recoveryCommitState),
) RecoveryCommit {
	state := cloneRecoveryCommitState(commit.state)
	mutate(&state)
	return RecoveryCommit{state: state}
}

func (r *recoveryStoreStub) ListNonTerminalRuns(context.Context) ([]rundomain.Run, error) {
	return append([]rundomain.Run(nil), r.runs...), nil
}

func (r *recoveryStoreStub) ListPendingInterrupts(context.Context) ([]Pending, error) {
	if r.aliasPending {
		return r.pending, nil
	}
	return append([]Pending(nil), r.pending...), nil
}

func (r *recoveryStoreStub) ListOpenModelInvocations(context.Context) ([]OpenModelInvocation, error) {
	if r.aliasOpen {
		return r.models, nil
	}
	return append([]OpenModelInvocation(nil), r.models...), nil
}

func (r *recoveryStoreStub) ListOpenToolInvocations(context.Context) ([]OpenToolInvocation, error) {
	if r.aliasOpen {
		return r.tools, nil
	}
	return append([]OpenToolInvocation(nil), r.tools...), nil
}

func (r *recoveryStoreStub) SessionByID(_ context.Context, sessionID string) (session.Session, error) {
	if sess, ok := r.sessions[sessionID]; ok {
		return sess, nil
	}
	return testsupport.MustRestoreSession(session.Snapshot{ID: sessionID, Workspace: testsupport.MustWorkspace("/workspace")}), nil
}

func TestRecoveryPlannerProtectsSessionPointRead(t *testing.T) {
	tests := []struct {
		name  string
		value session.Session
	}{
		{name: "invalid aggregate"},
		{
			name: "mismatched identity",
			value: testsupport.MustRestoreSession(session.Snapshot{
				ID: "ses_other", Workspace: testsupport.MustWorkspace("/workspace"),
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &recoveryStoreStub{sessions: map[string]session.Session{"ses_1": test.value}}
			planner := &recoveryPlanner{ctx: t.Context(), store: store, sessions: make(map[string]session.Session)}
			if _, err := planner.session("ses_1"); !errors.Is(err, session.ErrInvalid) {
				t.Fatalf("session error = %v, want ErrInvalid", err)
			}
		})
	}
}

func (r *recoveryStoreStub) ListTranscript(_ context.Context, sessionID string) ([]transcript.Item, error) {
	r.transcriptReads++
	return append([]transcript.Item(nil), r.transcripts[sessionID]...), nil
}

func (r *recoveryStoreStub) CountMessages(_ context.Context, sessionID string) (int, error) {
	if _, explicit := r.messageMarks[sessionID]; !explicit {
		return len(r.messages[sessionID]), nil
	}
	return r.messageMarks[sessionID], nil
}

func (r *recoveryStoreStub) ReadMessages(
	_ context.Context,
	sessionID string,
) ([]corechat.Message, error) {
	r.conversationReads++
	if messages, explicit := r.messages[sessionID]; explicit {
		cloned := make([]corechat.Message, len(messages))
		for index, message := range messages {
			cloned[index] = message.Clone()
		}
		return cloned, nil
	}
	messages := make([]corechat.Message, r.messageMarks[sessionID])
	for index := range messages {
		messages[index] = corechat.NewUserMessage(corechat.NewTextPart(fmt.Sprintf("message %d", index+1)))
	}
	return messages, nil
}

func TestRecoveryRejectsInvalidTranscriptBeforePlanning(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	active := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_active", SessionID: "session_active", State: rundomain.Running,
		ActiveSegmentID: "segment_active", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	foreign := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_foreign", SessionID: "session_foreign", RunID: active.ID(),
		Kind: transcript.QuestionItem, OccurredAt: createdAt.Add(time.Hour),
		Question: &transcript.Question{
			Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}},
		},
	})
	duplicate := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_duplicate", SessionID: active.SessionID(), RunID: active.ID(),
		Kind: transcript.QuestionItem, OccurredAt: createdAt,
		Question: &transcript.Question{
			Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}},
		},
	})
	orphanRunning := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_orphan", SessionID: active.SessionID(), RunID: "run_terminal",
		Kind: transcript.ToolCall, Status: transcript.ItemRunning, OccurredAt: createdAt,
	})
	for name, items := range map[string][]transcript.Item{
		"invalid aggregate":                 {{}},
		"foreign Session":                   {foreign},
		"duplicate identity":                {duplicate, duplicate},
		"Running Item without active owner": {orphanRunning},
	} {
		t.Run(name, func(t *testing.T) {
			store := &recoveryStoreStub{
				runs:         []rundomain.Run{active},
				transcripts:  map[string][]transcript.Item{active.SessionID(): items},
				messageMarks: map[string]int{},
			}
			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
				func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
			))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}

			if _, err := recovery.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted an invalid transcript")
			}
			if store.conversationReads != 0 || store.commits != 0 {
				t.Fatalf(
					"invalid transcript reached planning or commit: conversationReads=%d commits=%d",
					store.conversationReads,
					store.commits,
				)
			}
		})
	}
}

func TestRecoveryRejectsActiveOpenToolWithoutItsRunningItem(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 5, 0, 0, 0, time.UTC)
	root := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_root", SessionID: "session", State: rundomain.Running,
		ActiveSegmentID: "segment_root", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	child := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_child", SessionID: root.SessionID(), State: rundomain.Running,
		ActiveSegmentID: "segment_child", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
		Lineage: rundomain.Lineage{
			ParentRunID: root.ID(), RootRunID: root.ID(), SpawnedByItemID: "item_spawn",
		},
	})
	open := OpenToolInvocation{
		SessionID: root.SessionID(), RunID: root.ID(), SegmentID: root.ActiveSegmentID(),
		CallID: "tool_open", ItemID: "item_open", StartedAt: createdAt,
	}
	terminal := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: open.ItemID, SessionID: root.SessionID(), RunID: root.ID(),
		Kind: transcript.ToolCall, Status: transcript.ItemIncomplete,
		OccurredAt: createdAt, FinishedAt: createdAt.Add(time.Second),
		Tool: &transcript.ToolInvocation{Name: "shell"},
	})
	wrongOwner := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: open.ItemID, SessionID: root.SessionID(), RunID: child.ID(),
		Kind: transcript.ToolCall, Status: transcript.ItemRunning, OccurredAt: createdAt,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	})
	for name, items := range map[string][]transcript.Item{
		"missing Item":    nil,
		"terminal Item":   {terminal},
		"different owner": {wrongOwner},
	} {
		t.Run(name, func(t *testing.T) {
			store := &recoveryStoreStub{
				runs: []rundomain.Run{root, child}, tools: []OpenToolInvocation{open},
				transcripts:  map[string][]transcript.Item{root.SessionID(): items},
				messageMarks: map[string]int{},
			}
			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
				func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
			))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}

			if _, err := recovery.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted an active open Tool without its Running Item")
			}
			if store.transcriptReads != 1 || store.conversationReads != 0 || store.commits != 0 {
				t.Fatalf(
					"invalid Tool/Item relation escaped admission: transcriptReads=%d conversationReads=%d commits=%d",
					store.transcriptReads,
					store.conversationReads,
					store.commits,
				)
			}
		})
	}
}

func (r *recoveryStoreStub) LoadExecutorCheckpoint(
	_ context.Context,
	rootMemberID string,
) (ExecutorCheckpoint, error) {
	if r.checkpointErr != nil {
		return ExecutorCheckpoint{}, r.checkpointErr
	}
	if r.checkpoint != nil {
		return r.checkpoint.Clone(), nil
	}
	for _, pending := range r.pending {
		root, found := pending.RootContinuation()
		if !found || root.MemberID != rootMemberID {
			continue
		}
		sess, found := r.sessions[pending.SessionID]
		if !found {
			sess = testsupport.MustRestoreSession(session.Snapshot{ID: pending.SessionID, Workspace: testsupport.MustWorkspace("/workspace")})
		}
		return ExecutorCheckpoint{
			RootMemberID: rootMemberID,
			Payload:      []byte(`{}`),
			BuildID:      testExecutorBuildID,
			Scope: ExecutionScope{
				SessionID: pending.SessionID, CWD: sess.Workspace().Path(), WorkspaceCWD: sess.Workspace().Path(),
				Isolated: sess.Isolated(), GoalIncarnationID: pending.GoalIncarnationID,
			},
			ModelSelection: root.ModelSelection,
			Limits:         root.Limits,
			Capabilities:   pending.Capabilities,
		}, nil
	}
	return ExecutorCheckpoint{}, ErrExecutorCheckpointNotFound
}

func (r *recoveryStoreStub) CommitRecovery(_ context.Context, commit RecoveryCommit) error {
	r.commits++
	r.commit = commit
	if r.mutateCommitViews {
		if values := commit.LostRuns(); len(values) != 0 {
			values[0] = rundomain.Replacement{}
		}
		if values := commit.ModelInvocations(); len(values) != 0 {
			values[0].SessionID = "session_mutated"
		}
		if values := commit.DeleteInterrupts(); len(values) != 0 {
			values[0].SessionID = "session_mutated"
		}
		if values := commit.DeleteCheckpointSessionIDs(); len(values) != 0 {
			values[0] = "session_mutated"
		}
		if values := commit.ConversationTransitions(); len(values) != 0 {
			values[0].SessionID = "session_mutated"
			if len(values[0].Messages) != 0 {
				values[0].Messages[0].Parts[0].ToolResult.Name = "mutated"
			}
		}
	}
	return r.commitErr
}

type waitingExecutionResumabilityFunc func(context.Context, WaitingContinuation) (bool, error)

func (w waitingExecutionResumabilityFunc) CanResumeWaitingExecution(
	ctx context.Context,
	continuation WaitingContinuation,
) (bool, error) {
	return w(ctx, continuation)
}

type selectiveRecoveryAdmissions struct {
	busy     map[string]bool
	released map[string]int
	acquired []string
}

func (s *selectiveRecoveryAdmissions) AcquireSession(sessionID string) (func(), bool, error) {
	s.acquired = append(s.acquired, sessionID)
	if s.busy[sessionID] {
		return nil, false, nil
	}
	return func() { s.released[sessionID]++ }, true, nil
}

func TestRecoveryRejectsInvalidRunCatalogBeforeAdmission(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	active := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_active", SessionID: "session_active", State: rundomain.Running,
		ActiveSegmentID: "segment_active", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	terminal, err := active.Terminate(rundomain.Termination{
		Outcome: rundomain.OutcomeCompleted, FinishedAt: createdAt.Add(time.Second), MessageMark: 0,
	})
	if err != nil {
		t.Fatalf("Terminate fixture: %v", err)
	}
	secondRoot := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_second", SessionID: active.SessionID(), State: rundomain.Running,
		ActiveSegmentID: "segment_second", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	for name, candidates := range map[string][]rundomain.Run{
		"invalid aggregate":          {{}},
		"terminal row":               {terminal},
		"duplicate Run identity":     {active, active},
		"multiple roots for Session": {active, secondRoot},
	} {
		t.Run(name, func(t *testing.T) {
			store := &recoveryStoreStub{runs: candidates}
			admissions := &selectiveRecoveryAdmissions{released: map[string]int{}}
			recovery, err := NewRecovery(
				store,
				waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
					return false, nil
				}),
				admissions,
				nil,
			)
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}

			if _, err := recovery.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted an invalid Run catalog")
			}
			if len(admissions.acquired) != 0 || store.transcriptReads != 0 || store.commits != 0 {
				t.Fatalf(
					"invalid catalog reached admission or planning: acquired=%v transcriptReads=%d commits=%d",
					admissions.acquired,
					store.transcriptReads,
					store.commits,
				)
			}
		})
	}
}

func TestRecoveryRejectsIncoherentActiveTreeBeforePlanning(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 0, 30, 0, 0, time.UTC)
	capabilities := rundomain.Capabilities{ChildRuns: true}
	root := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_root", SessionID: "session_tree", State: rundomain.Running,
		ActiveSegmentID: "segment_root", Capabilities: capabilities, CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	child := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_child", SessionID: root.SessionID(), State: rundomain.Running,
		ActiveSegmentID: "segment_child", Capabilities: capabilities, CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
		Lineage: rundomain.Lineage{
			SpawnedByItemID: "item_spawn", ParentRunID: root.ID(), RootRunID: root.ID(),
		},
	})
	for name, mutate := range map[string]func(*rundomain.Snapshot){
		"mixed lifecycle": func(snapshot *rundomain.Snapshot) {
			snapshot.State = rundomain.Waiting
			snapshot.ActiveSegmentID = ""
		},
		"capability drift": func(snapshot *rundomain.Snapshot) {
			snapshot.Capabilities = rundomain.Capabilities{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := child.Snapshot()
			mutate(&snapshot)
			candidate := testsupport.MustRestoreRun(snapshot)
			store := &recoveryStoreStub{
				runs:        []rundomain.Run{root, candidate},
				transcripts: map[string][]transcript.Item{}, messageMarks: map[string]int{},
			}
			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
				func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
			))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}

			if _, err := recovery.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted an incoherent active Run tree")
			}
			if store.transcriptReads != 0 || store.commits != 0 {
				t.Fatalf(
					"incoherent tree reached planning or commit: transcriptReads=%d commits=%d",
					store.transcriptReads,
					store.commits,
				)
			}
		})
	}
}

func TestNewRecoveryRejectsTypedNilDependencies(t *testing.T) {
	validStore := &recoveryStoreStub{}
	validResumability := waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		return true, nil
	})
	validAdmissions := &selectiveRecoveryAdmissions{}
	var typedNilStore *recoveryStoreStub
	var typedNilAdmissions *selectiveRecoveryAdmissions

	tests := []struct {
		name         string
		store        RecoveryStore
		resumability WaitingExecutionResumability
		admissions   RecoveryAdmissions
	}{
		{name: "store", store: typedNilStore, resumability: validResumability, admissions: validAdmissions},
		{name: "resumability", store: validStore, resumability: waitingExecutionResumabilityFunc(nil), admissions: validAdmissions},
		{name: "admissions", store: validStore, resumability: validResumability, admissions: typedNilAdmissions},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recovery, err := NewRecovery(test.store, test.resumability, test.admissions, nil)
			if err == nil || recovery != nil {
				t.Fatalf("NewRecovery = %#v, %v", recovery, err)
			}
		})
	}
}

func TestRecoverySkipsFactsOwnedByAnotherRuntime(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	recoverable := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_recoverable", SessionID: "session_recoverable", State: rundomain.Running,
		ActiveSegmentID: "segment_recoverable", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	foreign := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_foreign", SessionID: "session_foreign", State: rundomain.Running,
		ActiveSegmentID: "segment_foreign", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	store := &recoveryStoreStub{
		runs:              []rundomain.Run{recoverable, foreign},
		mutateCommitViews: true,
		models: []OpenModelInvocation{
			{SessionID: recoverable.SessionID(), RunID: recoverable.ID(), SegmentID: "segment_recoverable", CallID: "call_recoverable", StartedAt: createdAt},
			{SessionID: foreign.SessionID(), RunID: foreign.ID(), SegmentID: "segment_foreign", CallID: "call_foreign", StartedAt: createdAt},
		},
		transcripts:  map[string][]transcript.Item{},
		messageMarks: map[string]int{},
	}
	admissions := &selectiveRecoveryAdmissions{
		busy: map[string]bool{foreign.SessionID(): true}, released: map[string]int{},
	}
	var notices []invalidation.Notice
	recovery, err := NewRecovery(
		store,
		waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
			return true, nil
		}),
		admissions,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovery.now = func() time.Time { return createdAt.Add(time.Minute) }
	reconciled, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	lostRuns := store.commit.LostRuns()
	if reconciled != 1 || len(lostRuns) != 1 || lostRuns[0].State().ID() != recoverable.ID() {
		t.Fatalf("recovery touched wrong Runs: reconciled=%d lost=%+v", reconciled, lostRuns)
	}
	modelInvocations := store.commit.ModelInvocations()
	if len(modelInvocations) != 1 || modelInvocations[0].RunID != recoverable.ID() {
		t.Fatalf("recovery touched foreign invocation: %+v", modelInvocations)
	}
	if !reflect.DeepEqual(store.commit.RecoveredSessionIDs(), []string{recoverable.SessionID()}) ||
		!reflect.DeepEqual(store.commit.DeleteCheckpointSessionIDs(), []string{recoverable.SessionID()}) {
		t.Fatalf("recovery cleanup scope = sessions:%v checkpoints:%v", store.commit.RecoveredSessionIDs(), store.commit.DeleteCheckpointSessionIDs())
	}
	if admissions.released[recoverable.SessionID()] != 1 || admissions.released[foreign.SessionID()] != 0 {
		t.Fatalf("recovery releases = %+v", admissions.released)
	}
	wantNotices := []invalidation.Notice{
		invalidation.InSession(invalidation.Runs, recoverable.SessionID(), recoverable.ID()),
		invalidation.InSession(invalidation.Interrupts, recoverable.SessionID(), recoverable.ID()),
		invalidation.InSession(invalidation.Sessions, recoverable.SessionID()),
	}
	if !reflect.DeepEqual(notices, wantNotices) {
		t.Fatalf("recovery notices = %+v, want %+v", notices, wantNotices)
	}
}

func TestRecoveryOwnsOpenInvocationCatalogsBeforeFiltering(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC)
	active := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_active", SessionID: "session_active", State: rundomain.Running,
		ActiveSegmentID: "segment_active", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	models := []OpenModelInvocation{
		{SessionID: "session_foreign", RunID: "run_foreign", SegmentID: "segment_foreign", CallID: "model_foreign", StartedAt: createdAt},
		{SessionID: active.SessionID(), RunID: active.ID(), SegmentID: "segment_active", CallID: "model_active", StartedAt: createdAt},
	}
	tools := []OpenToolInvocation{
		{SessionID: "session_foreign", RunID: "run_foreign", SegmentID: "segment_foreign", CallID: "tool_foreign", ItemID: "item_foreign", StartedAt: createdAt},
		{SessionID: active.SessionID(), RunID: active.ID(), SegmentID: "segment_active", CallID: "tool_active", ItemID: "item_active", StartedAt: createdAt},
	}
	activeToolItem := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_active", SessionID: active.SessionID(), RunID: active.ID(),
		Kind: transcript.ToolCall, Status: transcript.ItemRunning, OccurredAt: createdAt,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	})
	wantModels := append([]OpenModelInvocation(nil), models...)
	wantTools := append([]OpenToolInvocation(nil), tools...)
	store := &recoveryStoreStub{
		runs: []rundomain.Run{active}, models: models, tools: tools, aliasOpen: true,
		transcripts:  map[string][]transcript.Item{active.SessionID(): {activeToolItem}},
		messageMarks: map[string]int{},
	}
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
		func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
	))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovery.now = func() time.Time { return createdAt.Add(time.Minute) }

	if _, err := recovery.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !reflect.DeepEqual(store.models, wantModels) || !reflect.DeepEqual(store.tools, wantTools) {
		t.Fatalf("recovery mutated store-owned catalogs: models=%+v tools=%+v", store.models, store.tools)
	}
}

func TestRecoveryOwnsPendingCatalogBeforeFiltering(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	catalog := []Pending{{RootRunID: "run_foreign"}, pending}
	want := append([]Pending(nil), catalog...)
	store := &recoveryStoreStub{
		runs: []rundomain.Run{run}, pending: catalog, aliasPending: true,
		transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
	}
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
		func(context.Context, WaitingContinuation) (bool, error) { return true, nil },
	))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !reflect.DeepEqual(store.pending, want) {
		t.Fatalf("recovery mutated store-owned Pending catalog: got %+v, want %+v", store.pending, want)
	}
}

func TestRecoveryRejectsInvalidClaimedPendingBeforePlanning(t *testing.T) {
	_, pending, _ := coherentRecoveryPark(t)
	rootContinuation, ok := pending.RootContinuation()
	if !ok {
		t.Fatal("Pending fixture has no root continuation")
	}
	pending.Interrupts[0].ItemID = ""
	active := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: pending.RootRunID, SessionID: pending.SessionID, State: rundomain.Running,
		ActiveSegmentID: "segment_active", ModelSelection: rootContinuation.ModelSelection,
		Capabilities: pending.Capabilities, CreatedAt: rootContinuation.RunCreatedAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	store := &recoveryStoreStub{
		runs: []rundomain.Run{active}, pending: []Pending{pending},
		transcripts: map[string][]transcript.Item{}, messageMarks: map[string]int{},
	}
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
		func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
	))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); err == nil {
		t.Fatal("Reconcile accepted an invalid claimed Pending catalog")
	}
	if store.transcriptReads != 0 || store.commits != 0 {
		t.Fatalf("invalid Pending reached planning or commit: transcriptReads=%d commits=%d", store.transcriptReads, store.commits)
	}
}

func TestRecoveryRejectsInvalidClaimedOpenInvocationsBeforePlanning(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	active := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_active", SessionID: "session_active", State: rundomain.Running,
		ActiveSegmentID: "segment_active", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	model := OpenModelInvocation{
		SessionID: active.SessionID(), RunID: active.ID(), SegmentID: "segment_active",
		CallID: "model_active", StartedAt: createdAt,
	}
	toolInvocation := OpenToolInvocation{
		SessionID: active.SessionID(), RunID: active.ID(), SegmentID: "segment_active",
		CallID: "tool_active", ItemID: "item_active", StartedAt: createdAt,
	}
	for name, input := range map[string]struct {
		models []OpenModelInvocation
		tools  []OpenToolInvocation
	}{
		"missing model call": {models: []OpenModelInvocation{{
			SessionID: model.SessionID, RunID: model.RunID, SegmentID: model.SegmentID, StartedAt: model.StartedAt,
		}}},
		"non-UTC model start": {models: []OpenModelInvocation{{
			SessionID: model.SessionID, RunID: model.RunID, SegmentID: model.SegmentID,
			CallID: model.CallID, StartedAt: model.StartedAt.In(time.FixedZone("offset", 60)),
		}}},
		"duplicate model call": {models: []OpenModelInvocation{model, model}},
		"missing Tool Item": {tools: []OpenToolInvocation{{
			SessionID: toolInvocation.SessionID, RunID: toolInvocation.RunID,
			SegmentID: toolInvocation.SegmentID, CallID: toolInvocation.CallID,
			StartedAt: toolInvocation.StartedAt,
		}}},
		"duplicate Tool Item": {tools: []OpenToolInvocation{
			toolInvocation,
			{
				SessionID: toolInvocation.SessionID, RunID: toolInvocation.RunID,
				SegmentID: toolInvocation.SegmentID, CallID: "tool_second",
				ItemID: toolInvocation.ItemID, StartedAt: toolInvocation.StartedAt,
			},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			store := &recoveryStoreStub{
				runs: []rundomain.Run{active}, models: input.models, tools: input.tools,
				transcripts: map[string][]transcript.Item{}, messageMarks: map[string]int{},
			}
			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
				func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
			))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}
			if _, err := recovery.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted an invalid claimed open-invocation catalog")
			}
			if store.transcriptReads != 0 || store.commits != 0 {
				t.Fatalf("invalid catalog reached planning or commit: transcriptReads=%d commits=%d", store.transcriptReads, store.commits)
			}
		})
	}
}

func TestRecoveryRejectsOpenInvocationThatContradictsActiveRun(t *testing.T) {
	createdAt := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	first := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_first", SessionID: "session_first", State: rundomain.Running,
		ActiveSegmentID: "segment_first", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	second := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_second", SessionID: "session_second", State: rundomain.Running,
		ActiveSegmentID: "segment_second", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	model := OpenModelInvocation{
		SessionID: first.SessionID(), RunID: first.ID(), SegmentID: first.ActiveSegmentID(),
		CallID: "model_active", StartedAt: createdAt,
	}
	toolInvocation := OpenToolInvocation{
		SessionID: first.SessionID(), RunID: first.ID(), SegmentID: first.ActiveSegmentID(),
		CallID: "tool_active", ItemID: "item_active", StartedAt: createdAt,
	}
	for name, input := range map[string]struct {
		models []OpenModelInvocation
		tools  []OpenToolInvocation
	}{
		"model Session": {models: []OpenModelInvocation{{
			SessionID: second.SessionID(), RunID: model.RunID, SegmentID: model.SegmentID,
			CallID: model.CallID, StartedAt: model.StartedAt,
		}}},
		"model Segment": {models: []OpenModelInvocation{{
			SessionID: model.SessionID, RunID: model.RunID, SegmentID: second.ActiveSegmentID(),
			CallID: model.CallID, StartedAt: model.StartedAt,
		}}},
		"Tool Session": {tools: []OpenToolInvocation{{
			SessionID: second.SessionID(), RunID: toolInvocation.RunID, SegmentID: toolInvocation.SegmentID,
			CallID: toolInvocation.CallID, ItemID: toolInvocation.ItemID, StartedAt: toolInvocation.StartedAt,
		}}},
		"Tool Segment": {tools: []OpenToolInvocation{{
			SessionID: toolInvocation.SessionID, RunID: toolInvocation.RunID, SegmentID: second.ActiveSegmentID(),
			CallID: toolInvocation.CallID, ItemID: toolInvocation.ItemID, StartedAt: toolInvocation.StartedAt,
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			store := &recoveryStoreStub{
				runs: []rundomain.Run{first, second}, models: input.models, tools: input.tools,
				transcripts: map[string][]transcript.Item{}, messageMarks: map[string]int{},
			}
			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
				func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
			))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}

			if _, err := recovery.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted an open invocation that contradicts its active Run")
			}
			if store.transcriptReads != 0 || store.commits != 0 {
				t.Fatalf(
					"contradictory invocation reached planning or commit: transcriptReads=%d commits=%d",
					store.transcriptReads,
					store.commits,
				)
			}
		})
	}
}

func TestRecoveryDoesNotPublishBeforeItsCommitSucceeds(t *testing.T) {
	createdAt := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	abandoned := testsupport.MustRestoreRun(rundomain.Snapshot{
		ID: "run_abandoned", SessionID: "session_abandoned", State: rundomain.Running,
		ActiveSegmentID: "segment_abandoned", CreatedAt: createdAt,
		MessageMark: rundomain.UnknownMessageMark,
	})
	commitErr := errors.New("commit failed")
	store := &recoveryStoreStub{
		runs: []rundomain.Run{abandoned}, transcripts: map[string][]transcript.Item{},
		messageMarks: map[string]int{}, commitErr: commitErr,
	}
	admissions := &selectiveRecoveryAdmissions{released: map[string]int{}}
	var notices []invalidation.Notice
	recovery, err := NewRecovery(
		store,
		waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
			return true, nil
		}),
		admissions,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	if _, err := recovery.Reconcile(t.Context()); !errors.Is(err, commitErr) {
		t.Fatalf("Reconcile error = %v, want %v", err, commitErr)
	}
	if len(notices) != 0 {
		t.Fatalf("failed recovery published notices: %+v", notices)
	}
	if admissions.released[abandoned.SessionID()] != 1 {
		t.Fatalf("failed recovery releases = %+v, want exact claimed Session once", admissions.released)
	}
}

func TestRecoveryMarksAbandonedRunTreeLostInPostorder(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	root := testsupport.MustRestoreRun(rundomain.Snapshot{ID: "run_root", SessionID: "session", State: rundomain.Running,
		ActiveSegmentID: "segment_root", CreatedAt: createdAt, MessageMark: rundomain.UnknownMessageMark})

	child := testsupport.MustRestoreRun(rundomain.Snapshot{ID: "run_child", SessionID: root.SessionID(), State: rundomain.Running,
		ActiveSegmentID: "segment_child",
		CreatedAt:       createdAt, MessageMark: rundomain.UnknownMessageMark, Lineage: rundomain.Lineage{ParentRunID: root.ID(), RootRunID: root.ID(),
			SpawnedByItemID: "item_spawn"}})

	item := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_running", SessionID: root.SessionID(), RunID: child.ID(),
		Kind: transcript.QuestionItem, OccurredAt: createdAt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}}},
	})
	toolItem := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_tool_child", SessionID: root.SessionID(), RunID: child.ID(),
		Kind: transcript.ToolCall, Status: transcript.ItemRunning, OccurredAt: createdAt,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	})
	store := &recoveryStoreStub{
		runs: []rundomain.Run{root, child},
		models: []OpenModelInvocation{
			{SessionID: root.SessionID(), RunID: root.ID(), SegmentID: "segment_root", CallID: "model_root", StartedAt: createdAt.Add(time.Second)},
			{SessionID: root.SessionID(), RunID: "run_already_terminal", SegmentID: "segment_old", CallID: "model_orphan", StartedAt: createdAt.Add(2 * time.Second)},
		},
		tools: []OpenToolInvocation{{
			SessionID: child.SessionID(), RunID: child.ID(), SegmentID: "segment_child",
			CallID: "tool_child", ItemID: "item_tool_child", StartedAt: createdAt.Add(3 * time.Second),
		}},
		transcripts:  map[string][]transcript.Item{root.SessionID(): {item, toolItem}},
		messageMarks: map[string]int{root.SessionID(): 7},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovery.now = func() time.Time { return finishedAt }

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 2 || store.commits != 1 || checkpointCalls != 0 {
		t.Fatalf("recovered/commits/checkpointCalls = %d/%d/%d, want 2/1/0", recovered, store.commits, checkpointCalls)
	}
	lostRuns := store.commit.LostRuns()
	if got := []string{lostRuns[0].State().ID(), lostRuns[1].State().ID()}; !reflect.DeepEqual(got, []string{child.ID(), root.ID()}) {
		t.Fatalf("lost Run order = %v, want child-before-parent", got)
	}
	if !lostRuns[0].Expected().Equal(child) ||
		!lostRuns[1].Expected().Equal(root) {
		t.Fatalf("lost Run replacements discarded their exact pre-recovery states: %+v", lostRuns)
	}
	for _, replacement := range lostRuns {
		lost := replacement.State()
		if lost.State() != rundomain.Failed || !runHasOutcome(lost, rundomain.OutcomeLost) ||
			!runHasFailureKind(lost, rundomain.FailureLost) ||
			lost.MessageMark() != 7 || !lost.FinishedAt().Equal(finishedAt) {
			t.Fatalf("lost Run = %+v", lost)
		}
	}
	itemReplacements := store.commit.ItemReplacements()
	if len(itemReplacements) != 1 ||
		itemReplacements[0].Expected().ID() != toolItem.ID() ||
		itemReplacements[0].State().Status() != transcript.ItemIncomplete {
		t.Fatalf("Item replacements = %+v, want only the open Tool Item abandoned", itemReplacements)
	}
	if preserved := store.commit.PreservedSessionIDs(); len(preserved) != 0 {
		t.Fatalf("preserved Sessions = %v, want none", preserved)
	}
	modelInvocations := store.commit.ModelInvocations()
	toolInvocations := store.commit.ToolInvocations()
	if len(modelInvocations) != 2 || len(toolInvocations) != 1 {
		t.Fatalf(
			"recovered invocations = model:%+v Tool:%+v",
			modelInvocations,
			toolInvocations,
		)
	}
	for _, invocation := range modelInvocations {
		if !invocation.FinishedAt.Equal(finishedAt) {
			t.Fatalf("model invocation recovery = %+v", invocation)
		}
	}
	if !toolInvocations[0].FinishedAt.Equal(finishedAt) {
		t.Fatalf("Tool invocation recovery = %+v", toolInvocations[0])
	}
	foreignOrphan := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		for index := range state.ModelInvocations {
			if state.ModelInvocations[index].RunID == "run_already_terminal" {
				state.ModelInvocations[index].SessionID = "session_foreign"
			}
		}
	})
	if err := foreignOrphan.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted an orphan invocation outside recovered Session ownership")
	}
	missingCheckpointDeletion := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		state.DeleteCheckpointSessionIDs = nil
	})
	if err := missingCheckpointDeletion.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a lost tree without checkpoint cleanup")
	}
	foreignCheckpointDeletion := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		state.DeleteCheckpointSessionIDs = append(state.DeleteCheckpointSessionIDs, "session_foreign")
	})
	if err := foreignCheckpointDeletion.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted checkpoint cleanup for an unrelated Session")
	}
	missingToolReplacement := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		state.ItemReplacements = nil
	})
	if err := missingToolReplacement.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a lost-Run Tool journal without its Item replacement")
	}
	wrongInvocationSegment := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		for index := range state.ModelInvocations {
			if state.ModelInvocations[index].RunID == root.ID() {
				state.ModelInvocations[index].SegmentID = "segment_wrong"
			}
		}
	})
	if err := wrongInvocationSegment.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted an invocation outside its recovered active Segment")
	}
}

func TestRecoveryDoesNotMoveDurableTimeBackwardWhenTheClockRegresses(t *testing.T) {
	base := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		updatedAt time.Time
		itemAt    time.Time
		modelAt   time.Time
		toolAt    time.Time
		want      time.Time
	}{
		{name: "Run update", updatedAt: base.Add(2 * time.Minute), want: base.Add(2 * time.Minute)},
		{name: "Transcript Item", itemAt: base.Add(3 * time.Minute), want: base.Add(3 * time.Minute)},
		{name: "model attempt", modelAt: base.Add(4 * time.Minute), want: base.Add(4 * time.Minute)},
		{name: "Tool attempt", toolAt: base.Add(5 * time.Minute), want: base.Add(5 * time.Minute)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updatedAt := test.updatedAt
			if updatedAt.IsZero() {
				updatedAt = base
			}
			active := testsupport.MustRestoreRun(rundomain.Snapshot{
				ID: "run", SessionID: "session", State: rundomain.Running,
				ActiveSegmentID: "segment", CreatedAt: base, UpdatedAt: updatedAt,
				MessageMark: rundomain.UnknownMessageMark,
			})
			store := &recoveryStoreStub{
				runs:         []rundomain.Run{active},
				transcripts:  map[string][]transcript.Item{},
				messageMarks: map[string]int{"session": 0},
			}
			if !test.itemAt.IsZero() {
				store.transcripts["session"] = []transcript.Item{testsupport.MustRestoreItem(testsupport.ItemInput{
					ID: "item", SessionID: "session", RunID: active.ID(),
					Kind: transcript.ToolCall, Status: transcript.ItemRunning,
					OccurredAt: test.itemAt,
					Tool:       &transcript.ToolInvocation{Name: "shell"},
				})}
			}
			if !test.modelAt.IsZero() {
				store.models = []OpenModelInvocation{{
					SessionID: "session", RunID: active.ID(), SegmentID: "segment",
					CallID: "model", StartedAt: test.modelAt,
				}}
			}
			if !test.toolAt.IsZero() {
				store.tools = []OpenToolInvocation{{
					SessionID: "session", RunID: active.ID(), SegmentID: "segment",
					CallID: "tool", ItemID: "tool_item", StartedAt: test.toolAt,
				}}
				store.transcripts["session"] = []transcript.Item{testsupport.MustRestoreItem(testsupport.ItemInput{
					ID: "tool_item", SessionID: "session", RunID: active.ID(),
					Kind: transcript.ToolCall, Status: transcript.ItemRunning,
					OccurredAt: base,
					Tool:       &transcript.ToolInvocation{Name: "shell"},
				})}
			}

			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(
				func(context.Context, WaitingContinuation) (bool, error) { return false, nil },
			))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}
			recovery.now = func() time.Time { return base.Add(-time.Minute) }

			if _, err := recovery.Reconcile(t.Context()); err != nil {
				t.Fatalf("Reconcile with regressed clock: %v", err)
			}
			if got := store.commit.LostRuns()[0].State().FinishedAt(); !got.Equal(test.want) {
				t.Fatalf("lost Run finish = %v, want durable high watermark %v", got, test.want)
			}
			for _, invocation := range store.commit.ModelInvocations() {
				if !invocation.FinishedAt.Equal(test.want) {
					t.Fatalf("model finish = %v, want %v", invocation.FinishedAt, test.want)
				}
			}
			for _, invocation := range store.commit.ToolInvocations() {
				if !invocation.FinishedAt.Equal(test.want) {
					t.Fatalf("Tool finish = %v, want %v", invocation.FinishedAt, test.want)
				}
			}
			for _, replacement := range store.commit.ItemReplacements() {
				if !replacement.State().FinishedAt().Equal(test.want) {
					t.Fatalf("Item finish = %v, want %v", replacement.State().FinishedAt(), test.want)
				}
			}
		})
	}
}

func TestRecoveryChargesLostGoalOwnedRootToItsAdmissionLease(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 1, 30, 0, 0, time.UTC)
	finishedAt := createdAt.Add(time.Minute)
	cost := 1.25
	run := testsupport.MustRestoreRun(rundomain.Snapshot{ID: "run_goal", SessionID: "session", State: rundomain.Running,
		ActiveSegmentID: "segment_goal", GoalIncarnationID: "lease_goal",
		Metrics: testsupport.MustRunMetrics(testsupport.RunMetricsInput{Steps: 3,
			Usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &cost}}}),

		CreatedAt: createdAt, MessageMark: rundomain.UnknownMessageMark})

	store := &recoveryStoreStub{
		runs:         []rundomain.Run{run},
		transcripts:  map[string][]transcript.Item{run.SessionID(): nil},
		messageMarks: map[string]int{run.SessionID(): 2},
	}
	var notices []invalidation.Notice
	recovery, err := NewRecovery(
		store,
		waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
			return false, nil
		}),
		testsupport.NewAdmissionGate(),
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovery.now = func() time.Time { return finishedAt }

	if _, err := recovery.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	goalRuns := store.commit.GoalRuns()
	if len(goalRuns) != 1 {
		t.Fatalf("Goal Runs = %+v, want one", goalRuns)
	}
	goalRun := goalRuns[0]
	goalCost, goalCostAvailable := goalRun.Cost.USD()
	if goalRun.SessionID != run.SessionID() || goalRun.IncarnationID != run.GoalIncarnationID() ||
		goalRun.RunID != run.ID() || goalRun.Outcome != rundomain.OutcomeLost ||
		!goalCostAvailable || goalCost != cost || goalRun.Steps != run.Metrics().Steps() ||
		!goalRun.CompletedAt.Equal(finishedAt) {
		t.Fatalf("Goal Run = %+v", goalRun)
	}
	wantNotices := []invalidation.Notice{
		invalidation.InSession(invalidation.Runs, run.SessionID(), run.ID()),
		invalidation.InSession(invalidation.Interrupts, run.SessionID(), run.ID()),
		invalidation.InSession(invalidation.Sessions, run.SessionID()),
		invalidation.InSession(invalidation.Goals, run.SessionID()),
	}
	if !reflect.DeepEqual(notices, wantNotices) {
		t.Fatalf("recovery notices = %+v, want %+v", notices, wantNotices)
	}

	missingCharge := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		state.GoalRuns = nil
	})
	if err := missingCharge.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a lost goal-owned Run without its charge")
	}
	mismatchedCharge := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		state.GoalRuns[0].IncarnationID = "other-lease"
	})
	if err := mismatchedCharge.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a Goal Run from another incarnation")
	}
	foreignDeletion := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		state.DeleteInterrupts = append(state.DeleteInterrupts, InterruptOwner{
			SessionID: "other-session", RootRunID: "run_foreign",
		})
	})
	if err := foreignDeletion.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted deletion of an unrelated Pending set")
	}
}

func TestRecoveryPreservesOnlyCoherentInterruptedTree(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	pending.GoalIncarnationID = "goal-lease-1"
	snapshot := run.Snapshot()
	snapshot.GoalIncarnationID = pending.GoalIncarnationID
	run = testsupport.MustRestoreRun(snapshot)
	store := &recoveryStoreStub{
		runs:         []rundomain.Run{run},
		pending:      []Pending{pending},
		transcripts:  map[string][]transcript.Item{run.SessionID(): {item}},
		messageMarks: map[string]int{run.SessionID(): 3},
	}
	var validated WaitingContinuation
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(_ context.Context, continuation WaitingContinuation) (bool, error) {
		validated = continuation
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	wantContinuation, err := waitingContinuationFromPending(pending, ExecutorCheckpoint{
		RootMemberID: "member_root", Payload: []byte(`{}`), BuildID: testExecutorBuildID,
		Scope: ExecutionScope{
			SessionID: run.SessionID(), CWD: "/workspace", WorkspaceCWD: "/workspace",
			GoalIncarnationID: pending.GoalIncarnationID,
		},
		ModelSelection: run.ModelSelection(), Limits: run.Limits(), Capabilities: pending.Capabilities,
	})
	if err != nil {
		t.Fatalf("waitingContinuationFromPending: %v", err)
	}
	if recovered != 0 || !reflect.DeepEqual(validated, wantContinuation) || len(store.commit.LostRuns()) != 0 {
		t.Fatalf("recovery = %d validated=%+v commit=%+v", recovered, validated, store.commit)
	}
	if !reflect.DeepEqual(store.commit.PreservedSessionIDs(), []string{run.SessionID()}) ||
		len(store.commit.DeleteInterrupts()) != 0 {
		t.Fatalf("ownership plan = %+v", store.commit)
	}
	if got := store.commit.RecoveredSessionIDs(); !reflect.DeepEqual(got, []string{run.SessionID()}) {
		t.Fatalf("recovered Session projection = %v, want preserved Session", got)
	}
}

func TestRecoveryPreservesQuestionToolWhileItsCheckpointIsResumable(t *testing.T) {
	run, pending, questionItem := coherentRecoveryPark(t)
	toolItem := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_tool", SessionID: run.SessionID(), RunID: run.ID(),
		Kind: transcript.ToolCall, Status: transcript.ItemRunning,
		OccurredAt: pending.CreatedAt,
		Tool:       &transcript.ToolInvocation{Name: "ask_user"},
	})
	pending.Continuations[0].DrainedTools = []DrainedTool{{
		ItemID: toolItem.ID(), ItemOccurredAt: toolItem.OccurredAt(),
		CallID: "tool:runtime:0", Name: "ask_user", Arguments: "{}",
	}}
	if err := pending.Validate(); err != nil {
		t.Fatalf("Pending fixture: %v", err)
	}
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {questionItem, toolItem}},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 0 || checkpointCalls != 1 || len(store.commit.LostRuns()) != 0 ||
		!reflect.DeepEqual(store.commit.PreservedSessionIDs(), []string{run.SessionID()}) {
		t.Fatalf("recovery = %d checkpointCalls=%d commit=%+v", recovered, checkpointCalls, store.commit)
	}
}

func TestRecoveryMarksIsolatedParkLostWithoutProbingExecutorCheckpoint(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
		sessions: map[string]session.Session{
			run.SessionID(): testsupport.MustRestoreSession(session.Snapshot{
				ID: run.SessionID(), Workspace: testsupport.MustWorkspace("/workspace"), Isolated: true,
			}),
		},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 1 || checkpointCalls != 0 || len(store.commit.LostRuns()) != 1 {
		t.Fatalf("recovered=%d checkpointCalls=%d commit=%+v", recovered, checkpointCalls, store.commit)
	}
}

func TestRecoveryTreatsUnavailableExecutorCheckpointAsResourceLoss(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:         []rundomain.Run{run},
		pending:      []Pending{pending},
		transcripts:  map[string][]transcript.Item{run.SessionID(): {item}},
		messageMarks: map[string]int{run.SessionID(): 5},
	}
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		return false, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 1 || len(store.commit.LostRuns()) != 1 ||
		!reflect.DeepEqual(store.commit.DeleteInterrupts(), []InterruptOwner{{SessionID: run.SessionID(), RootRunID: run.ID()}}) ||
		len(store.commit.PreservedSessionIDs()) != 0 {
		t.Fatalf("resource-loss recovery = %d, commit %+v", recovered, store.commit)
	}
}

func TestRecoveryTreatsInvalidExecutorCheckpointAsResourceLoss(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:          []rundomain.Run{run},
		pending:       []Pending{pending},
		transcripts:   map[string][]transcript.Item{run.SessionID(): {item}},
		checkpointErr: fmt.Errorf("corrupt durable policy: %w", ErrInvalidExecutorCheckpoint),
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(t.Context())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 1 || checkpointCalls != 0 || len(store.commit.LostRuns()) != 1 ||
		len(store.commit.PreservedSessionIDs()) != 0 {
		t.Fatalf("invalid-checkpoint recovery = %d checkpointCalls=%d commit=%+v", recovered, checkpointCalls, store.commit)
	}
}

func TestRecoveryRejectsExecutorCheckpointOwnedByDifferentApplicationFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ExecutorCheckpoint)
	}{
		{name: "root member", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.RootMemberID = "member_other"
		}},
		{name: "session", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.Scope.SessionID = "session_other"
		}},
		{name: "working directory", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.Scope.CWD = "/other/workspace"
		}},
		{name: "workspace", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.Scope.WorkspaceCWD = "/other/workspace"
		}},
		{name: "isolation", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.Scope.Isolated = true
		}},
		{name: "goal incarnation", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.Scope.GoalIncarnationID = "goal_other"
		}},
		{name: "provider", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.ModelSelection = mustCheckpointSelection("openai", checkpoint.ModelSelection.Model())
		}},
		{name: "model", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.ModelSelection = mustCheckpointSelection(checkpoint.ModelSelection.Provider(), "model_other")
		}},
		{name: "limits", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.Limits = testsupport.MustRunLimits(rundomain.LimitValues{MaxSteps: testsupport.Pointer(1)})
		}},
		{name: "capabilities", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.Capabilities.ChildRuns = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run, pending, item := coherentRecoveryPark(t)
			root, found := pending.RootContinuation()
			if !found {
				t.Fatal("coherent Pending has no root continuation")
			}
			checkpoint := ExecutorCheckpoint{
				RootMemberID: root.MemberID,
				Payload:      []byte(`{}`),
				BuildID:      testExecutorBuildID,
				Scope: ExecutionScope{
					SessionID:    run.SessionID(),
					CWD:          "/workspace",
					WorkspaceCWD: "/workspace",
				},
				ModelSelection: root.ModelSelection,
				Limits:         root.Limits,
				Capabilities:   pending.Capabilities.Clone(),
			}
			test.mutate(&checkpoint)

			store := &recoveryStoreStub{
				runs:        []rundomain.Run{run},
				pending:     []Pending{pending},
				transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
				checkpoint:  &checkpoint,
			}
			probeCalls := 0
			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
				probeCalls++
				return true, nil
			}))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}

			recovered, err := recovery.Reconcile(t.Context())
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if recovered != 1 || probeCalls != 0 || len(store.commit.LostRuns()) != 1 ||
				len(store.commit.PreservedSessionIDs()) != 0 {
				t.Fatalf(
					"mismatched checkpoint recovery=%d probeCalls=%d commit=%+v",
					recovered,
					probeCalls,
					store.commit,
				)
			}
		})
	}
}

func TestRecoveryAtomicallyClosesLostQuestionToolContext(t *testing.T) {
	run, pending, questionItem := coherentRecoveryPark(t)
	toolItem := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_tool", SessionID: run.SessionID(), RunID: run.ID(),
		Kind: transcript.ToolCall, Status: transcript.ItemRunning,
		OccurredAt: pending.CreatedAt,
		Tool:       &transcript.ToolInvocation{Name: "ask_user"},
	})
	pending.Continuations[0].DrainedTools = []DrainedTool{{
		ItemID: toolItem.ID(), ItemOccurredAt: toolItem.OccurredAt(),
		CallID: "tool:runtime:0", SourceCallID: "provider_call_open",
		Name: "ask_user", Arguments: "{}",
	}}
	conversation := []corechat.Message{
		corechat.NewUserMessage(corechat.NewTextPart("ask me")),
		corechat.NewAssistantMessage(corechat.NewToolCallPart(corechat.ToolCall{
			ID: "provider_call_open", Name: "ask_user", Arguments: "{}",
		})),
	}
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {questionItem, toolItem}},
		messages:    map[string][]corechat.Message{run.SessionID(): conversation},
		messageMarks: map[string]int{
			run.SessionID(): len(conversation),
		},
	}
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		return false, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	if _, err := recovery.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	conversationTransitions := store.commit.ConversationTransitions()
	itemReplacements := store.commit.ItemReplacements()
	lostRuns := store.commit.LostRuns()
	if len(conversationTransitions) != 1 || len(itemReplacements) != 1 {
		t.Fatalf("recovery commit = %+v", store.commit)
	}
	transition := conversationTransitions[0]
	if transition.RootRunID != run.ID() || transition.SessionID != run.SessionID() ||
		transition.ExpectedCount != 2 || len(transition.Messages) != 1 {
		t.Fatalf("conversation transition = %+v", transition)
	}
	result := transition.Messages[0].Parts[0].ToolResult
	if result == nil {
		t.Fatal("recovery closure has no Tool result")
	}
	resultText, textual := result.Output.Text()
	if result.ID != "provider_call_open" || result.Name != "ask_user" ||
		!textual || resultText != recoveryLostToolResult || !result.IsError ||
		lostRuns[0].State().MessageMark() != 3 {
		t.Fatalf("closure/lost Run = %#v / %+v", result, lostRuns[0])
	}

	missingClosure := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		state.ConversationTransitions = nil
	})
	if err := missingClosure.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a lost tree without its conversation transition")
	}
	wrongWatermark := invalidRecoveryCommit(store.commit, func(state *recoveryCommitState) {
		state.ConversationTransitions[0].ExpectedCount++
	})
	if err := wrongWatermark.Validate(); err == nil {
		t.Fatal("RecoveryCommit.Validate accepted a conversation watermark that differs from the lost Run")
	}
}

func TestRecoveryValidationFailureDoesNotCommitPartialRepair(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
	}
	want := errors.New("checkpoint backend unavailable")
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		return false, want
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); !errors.Is(err, want) {
		t.Fatalf("Reconcile error = %v, want %v", err, want)
	}
	if store.commits != 0 {
		t.Fatalf("CommitRecovery calls = %d, want none", store.commits)
	}
}

func TestRecoveryRejectsCrossSessionPendingWithoutCommit(t *testing.T) {
	run, pending, item := coherentRecoveryPark(t)
	pending.SessionID = "other-session"
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{run},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); err == nil {
		t.Fatal("Reconcile accepted a Pending owned by another Session")
	}
	if store.commits != 0 || checkpointCalls != 0 {
		t.Fatalf("recovery mutated or probed executor after corruption: commits=%d checkpointCalls=%d", store.commits, checkpointCalls)
	}
}

// TestRecoveryRejectsContinuationFactDriftWithoutProbingCheckpoint proves
// parked_continuation_matches_run_facts at boot recovery: contradictory facts
// fail before an executor probe or durable repair can turn them into history.
func TestRecoveryRejectsContinuationFactDriftWithoutProbingCheckpoint(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*rundomain.Run, *Pending)
	}{
		{
			name: "cumulative metrics",
			mutate: func(_ *rundomain.Run, pending *Pending) {
				metrics, err := rundomain.NewMetrics(nil, pending.Continuations[0].Metrics.Steps()+1, pending.Continuations[0].Metrics.ActiveDuration())
				if err != nil {
					panic(err)
				}
				pending.Continuations[0].Metrics = metrics
			},
		},
		{
			name: "frozen limits",
			mutate: func(_ *rundomain.Run, pending *Pending) {
				pending.Continuations[0].Limits = testsupport.MustRunLimits(rundomain.LimitValues{MaxSteps: testsupport.Pointer(1)})
			},
		},
		{
			name: "frozen run capabilities",
			mutate: func(run *rundomain.Run, _ *Pending) {
				snapshot := run.Snapshot()
				snapshot.Capabilities.ChildRuns = true
				*run = testsupport.MustRestoreRun(snapshot)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run, pending, item := coherentRecoveryPark(t)
			test.mutate(&run, &pending)
			store := &recoveryStoreStub{
				runs:        []rundomain.Run{run},
				pending:     []Pending{pending},
				transcripts: map[string][]transcript.Item{run.SessionID(): {item}},
			}
			checkpointCalls := 0
			recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
				checkpointCalls++
				return true, nil
			}))
			if err != nil {
				t.Fatalf("NewRecovery: %v", err)
			}

			if _, err := recovery.Reconcile(t.Context()); err == nil {
				t.Fatal("Reconcile accepted a continuation fact that differs from Run admission")
			}
			if store.commits != 0 || checkpointCalls != 0 {
				t.Fatalf("recovery mutated or probed after fact drift: commits=%d checkpointCalls=%d", store.commits, checkpointCalls)
			}
		})
	}
}

// TestRecoveryRejectsChildProtocolDriftWithoutProbingCheckpoint proves
// parked_continuation_matches_run_facts for root-owned policy: every child Run
// is parked under the root admission, even though Continuation does not repeat
// that policy as a second source of truth.
func TestRecoveryRejectsChildProtocolDriftWithoutProbingCheckpoint(t *testing.T) {
	root, pending, item := coherentRecoveryPark(t)
	rootSnapshot := root.Snapshot()
	rootSnapshot.Capabilities.ChildRuns = true
	root = testsupport.MustRestoreRun(rootSnapshot)
	pending.Capabilities.ChildRuns = true
	lineage := rundomain.Lineage{
		SpawnedByItemID: "item_spawn",
		ParentRunID:     root.ID(),
		RootRunID:       root.ID(),
	}
	child := testsupport.MustRestoreRun(rundomain.Snapshot{ID: "run_child", SessionID: root.SessionID(), State: rundomain.Waiting,

		ModelSelection: root.ModelSelection(),
		// This is a valid capabilities in isolation but contradicts the root admission.
		Capabilities: rundomain.Capabilities{
			InterruptKinds: []interruptdomain.Kind{interruptdomain.Question},
		},
		CreatedAt: root.CreatedAt(), MessageMark: rundomain.UnknownMessageMark, Lineage: rundomain.Lineage{SpawnedByItemID: lineage.SpawnedByItemID,
			ParentRunID: lineage.ParentRunID,
			RootRunID:   lineage.RootRunID}})

	rootContinuation := pending.Continuations[0]
	pending.Continuations = []Continuation{
		{
			RunID: "run_child", MemberID: "member_child",
			Lineage: lineage, ModelSelection: root.ModelSelection(),
			RunCreatedAt: root.CreatedAt(),
		},
		rootContinuation,
	}
	store := &recoveryStoreStub{
		runs:        []rundomain.Run{root, child},
		pending:     []Pending{pending},
		transcripts: map[string][]transcript.Item{root.SessionID(): {item}},
	}
	checkpointCalls := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, WaitingContinuation) (bool, error) {
		checkpointCalls++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(t.Context()); err == nil {
		t.Fatal("Reconcile accepted a child Run run capabilities that differs from root admission")
	}
	if store.commits != 0 || checkpointCalls != 0 {
		t.Fatalf("recovery mutated or probed after child policy drift: commits=%d checkpointCalls=%d", store.commits, checkpointCalls)
	}
}

func coherentRecoveryPark(t *testing.T) (rundomain.Run, Pending, transcript.Item) {
	t.Helper()
	createdAt := time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)
	selection, err := modelref.New("anthropic", "claude")
	if err != nil {
		t.Fatalf("model selection: %v", err)
	}
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}}}
	interrupt := transcript.Interrupt{
		ItemID: "item_question", ItemOccurredAt: createdAt,
		RunID: "run_root", Kind: interruptdomain.Question, Question: question,
	}
	run := testsupport.MustRestoreRun(rundomain.Snapshot{ID: "run_root", SessionID: "session", State: rundomain.Waiting,
		ModelSelection: selection,
		Capabilities:   rundomain.Capabilities{InterruptKinds: []interruptdomain.Kind{interruptdomain.Question}},
		CreatedAt:      createdAt, UpdatedAt: createdAt.Add(time.Second), MessageMark: rundomain.UnknownMessageMark})

	pending := Pending{
		RootRunID:  run.ID(),
		SessionID:  run.SessionID(),
		ExecutorID: "turn_root",
		Interrupts: []transcript.Interrupt{interrupt},
		Capabilities: rundomain.Capabilities{
			InterruptKinds: []interruptdomain.Kind{interruptdomain.Question},
		},
		Bindings: []InterruptBinding{{
			InterruptItemID: interrupt.ItemID,
			MemberID:        "member_root",
			RequestID:       "request_root",
		}},
		Continuations: []Continuation{{
			RunID: run.ID(), MemberID: "member_root", ModelSelection: selection, RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	if err := pending.Validate(); err != nil {
		t.Fatalf("Pending fixture: %v", err)
	}
	item := testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: interrupt.ItemID, SessionID: run.SessionID(), RunID: run.ID(),
		Kind:     transcript.QuestionItem,
		Question: question, OccurredAt: interrupt.ItemOccurredAt,
	})
	return run, pending, item
}
