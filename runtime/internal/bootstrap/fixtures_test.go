package bootstrap

import (
	"context"
	"testing"
	"time"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// sessionStores keeps the real durable collaborators visible to this integration
// fixture while delegating every tested write-set to the persistence adapter.
type sessionStores struct {
	*persistence.SessionStores
	sessions    *persistence.SessionStore
	transcript  *persistence.TranscriptStore
	interrupts  *persistence.InterruptStore
	runs        *persistence.RunStore
	checkpoints *persistence.ExecutorCheckpointStore
	history     *runsapp.ConversationHistory
	plan        *persistence.PlanStore
	approvals   *persistence.ApprovalRuleStore
	modes       *persistence.ModeStore
	toolResults *persistence.ToolResultStore
	childStarts *persistence.ChildRunStartReservationStore
	goals       *persistence.GoalStore
	retirements *persistence.ResourceRetirementStore
}

func bootstrapPending(
	runID, sessionID, memberID, itemID string,
	runCreatedAt, itemOccurredAt, barrierCreatedAt time.Time,
) runsapp.Pending {
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: questioncontract.Text}}}
	return runsapp.Pending{
		RootRunID:  runID,
		ExecutorID: "turn_" + runID,
		Interrupts: runsapp.OpenInterruptsOf([]transcript.Interrupt{{
			ItemID: itemID, ItemOccurredAt: itemOccurredAt,
			RunID:  runID,
			Prompt: *question,
		}}),
		Bindings: []runsapp.InterruptBinding{{
			InterruptItemID: itemID,
			MemberID:        memberID,
			RequestID:       "request-" + memberID,
		}},
		Continuations: []runsapp.Continuation{{
			RunID:    runID,
			MemberID: memberID,
		}},
		CreatedAt: barrierCreatedAt}
}

// bootstrapCheckpoint deliberately treats the executor payload as opaque. These
// Bootstrap fixtures verify Application write-sets, not Agent Framework wire;
// only adapter/run/execution may construct or interpret a tree snapshot.
func bootstrapCheckpoint(rootMemberID, sessionID string) runsapp.ExecutorCheckpoint {
	return runsapp.ExecutorCheckpoint{
		RootMemberID: rootMemberID,
		Payload:      []byte("opaque executor checkpoint"),
		BuildID:      bootstrapCheckpointBuildID,
		SessionID:    sessionID,
	}
}

const bootstrapCheckpointBuildID = testsupport.BuildID

func mustPlanCoordinator(deps sessions.PlanDependencies) *sessions.PlanCoordinator {
	if deps.Author == nil {
		deps.Author = testsupport.PlanAuthor{}
	}
	if deps.Invalidations == nil {
		deps.Invalidations = func(invalidation.Notice) {}
	}
	coordinator, err := sessions.NewPlanCoordinator(deps)
	if err != nil {
		panic(err)
	}
	return coordinator
}

func newWriteSetStores(t *testing.T, db *persistence.Bundle) (sessionStores, *persistence.RunStore, *persistence.InterruptStore) {
	t.Helper()
	runs := db.Runs
	ints := db.Interrupts
	plan := db.Plan
	approvals := db.ApprovalRules
	messages, err := persistence.NewConversationStore(db.ChatHistory)
	if err != nil {
		t.Fatal(err)
	}
	history, err := runsapp.NewConversationHistory(messages)
	if err != nil {
		t.Fatal(err)
	}
	ss := sessionStores{
		sessions:    db.Sessions,
		transcript:  db.Transcript,
		interrupts:  ints,
		runs:        runs,
		checkpoints: db.ExecutorCheckpoints,
		history:     history,
		plan:        plan,
		approvals:   approvals,
		modes:       db.ApprovalModes,
		toolResults: db.ToolResults,
		childStarts: db.ChildRunStarts,
		goals:       db.Goals,
		retirements: db.ResourceRetirements,
	}
	sessionStores, err := persistence.NewSessionStores(persistence.SessionStoresConfig{
		ResourceRetirements: db.ResourceRetirements,
		CommandAcceptances:  db.CommandAcceptances,
		Sessions:            ss.sessions, Transcript: ss.transcript, Interrupts: ss.interrupts,
		Runs: ss.runs, ExecutorCheckpoints: ss.checkpoints, History: ss.history, Plan: ss.plan,
		ApprovalRules: ss.approvals, ApprovalModes: ss.modes, ToolResults: ss.toolResults,
		ChildRunStarts: ss.childStarts, Goals: ss.goals,
		ReadSnapshot: func(ctx context.Context, read func(context.Context) error) error {
			return db.ReadSnapshot(ctx, read)
		},
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return db.Transactor(ctx, fn)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ss.SessionStores = sessionStores
	return ss, runs, ints
}

func bootstrapSession(id, title, cwd string) session.Session {
	return testsupport.MustRestoreSession(session.Snapshot{ID: id, Title: title, Workspace: testsupport.MustWorkspace(cwd)})
}

func insertBootstrapSession(t *testing.T, store *persistence.SessionStore, value session.Session) {
	t.Helper()
	if err := store.Insert(t.Context(), value); err != nil {
		t.Fatalf("insert Session %q: %v", value.ID(), err)
	}
}

// restoredRun is a finished Run as an import carries it — the only shape a
// restore accepts, since a Run that has not ended cannot be replayed into a
// session from the outside.
func restoredRun(sessionID, runID string, at time.Time) run.Run {
	outcome := run.OutcomeCompleted
	return testsupport.MustRestoreRun(run.Snapshot{
		SessionID: sessionID, ID: runID, State: run.Finished,
		Ending:    testsupport.EndingFor(outcome),
		CreatedAt: at, FinishedAt: at, UpdatedAt: at, MessageMark: run.MessageMarkAt(0),
	})
}
