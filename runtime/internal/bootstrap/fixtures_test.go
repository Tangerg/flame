package bootstrap

import (
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

type sessionStores struct {
	*persistence.SessionStores
	bundle  *persistence.Bundle
	history *runsapp.ConversationHistory
}

func bootstrapPending(
	runID, sessionID, memberID, itemID string,
	runCreatedAt, itemOccurredAt, barrierCreatedAt time.Time,
) runsapp.Pending {
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: questioncontract.Text}}}
	return runsapp.Pending{
		RootRunID:  testsupport.MustRunID(runID),
		ExecutorID: "turn_" + runID,
		Interrupts: runsapp.OpenInterruptsOf([]transcript.Interrupt{{
			ItemID: testsupport.MustItemID(itemID), ItemOccurredAt: itemOccurredAt,
			RunID:  testsupport.MustRunID(runID),
			Prompt: *question,
		}}),
		Bindings: []runsapp.InterruptBinding{{
			InterruptItemID: testsupport.MustItemID(itemID),
			MemberID:        memberID,
			RequestID:       "request-" + memberID,
		}},
		Continuations: []runsapp.Continuation{{
			RunID:    testsupport.MustRunID(runID),
			MemberID: memberID,
		}},
		CreatedAt: barrierCreatedAt}
}

func bootstrapCheckpoint(rootMemberID, sessionID string) runsapp.ExecutorCheckpoint {
	return runsapp.ExecutorCheckpoint{
		RootMemberID: rootMemberID,
		Payload:      []byte("opaque executor checkpoint"),
		BuildID:      bootstrapCheckpointBuildID,
		SessionID:    testsupport.MustSessionID(sessionID),
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

func newWriteSetStores(t *testing.T, db *persistence.Bundle) sessionStores {
	t.Helper()
	history, err := runsapp.NewConversationHistory(db.Conversations)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := db.SessionStores(history)
	if err != nil {
		t.Fatal(err)
	}
	return sessionStores{SessionStores: stores, bundle: db, history: history}
}

func bootstrapSession(id, title, cwd string) session.Session {
	return testsupport.MustRestoreSession(session.Snapshot{ID: testsupport.MustSessionID(id), Title: title, Workspace: testsupport.MustWorkspace(cwd)})
}

func insertBootstrapSession(t *testing.T, store *persistence.SessionStore, value session.Session) {
	t.Helper()
	if err := store.Insert(t.Context(), value); err != nil {
		t.Fatalf("insert Session %q: %v", value.ID(), err)
	}
}

func restoredRun(sessionID, runID string, at time.Time) run.Run {
	outcome := run.OutcomeCompleted
	return testsupport.MustRestoreRun(run.Snapshot{
		SessionID: testsupport.MustSessionID(sessionID), ID: testsupport.MustRunID(runID), State: run.Finished,
		Ending:    testsupport.EndingFor(outcome),
		CreatedAt: at, FinishedAt: at, UpdatedAt: at, MessageMark: run.MessageMarkAt(0),
	})
}
