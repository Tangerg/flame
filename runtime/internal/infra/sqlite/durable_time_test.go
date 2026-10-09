package sqlite_test

import (
	"math"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestDurableOwnerTimesRoundTripAtExactBounds(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).UTC(), time.Unix(0, 0).UTC(), time.Unix(0, math.MaxInt64).UTC()} {
		t.Run(at.Format(time.RFC3339Nano), func(t *testing.T) {
			db, err := sqlite.Open(t.Context(), ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			goals, plans, memory := sqlite.NewGoalStore(db), sqlite.NewPlanStore(db), sqlite.NewAgentMemoryStore(db)
			seedSession(t, sqlite.NewSessionStore(db), "session")
			value, err := goal.New("session", "objective", modelref.Selection{}, run.Capabilities{}, "incarnation", at)
			if err != nil {
				t.Fatal(err)
			}
			if saved, err := goals.Save(t.Context(), goalReplacement(t, value, unwrittenVersion(t, "session"))); err != nil || !saved {
				t.Fatalf("Save Goal = %v, %v", saved, err)
			}
			storedGoal, found, err := readGoal(t.Context(), goals, "session")
			if err != nil || !found || !storedGoal.CreatedAt().Equal(at) || !storedGoal.UpdatedAt().Equal(at) {
				t.Fatalf("Goal time = %+v, %v", storedGoal.Snapshot(), err)
			}
			state, err := (plan.Current{}).Replace(nil, at)
			if err != nil {
				t.Fatal(err)
			}
			change, err := plan.NewReplacement(plan.Version{}, state)
			if err != nil {
				t.Fatal(err)
			}
			if err := plans.Save(t.Context(), "session", change); err != nil {
				t.Fatal(err)
			}
			storedPlan, err := plans.State(t.Context(), "session")
			if err != nil || !committedPlan(t, storedPlan).UpdatedAt().Equal(at) {
				t.Fatalf("Plan time = %v, %v", committedPlan(t, storedPlan).UpdatedAt(), err)
			}
			item, added, err := memory.Add(t.Context(), agentmemory.ScopeUser, "", "user content", at)
			if err != nil || !added {
				t.Fatalf("Add = %v, %v", added, err)
			}
			storedItem, found, err := memory.Get(t.Context(), item.ID())
			if err != nil || !found || !storedItem.CreatedAt().Equal(at) || !storedItem.UpdatedAt().Equal(at) {
				t.Fatalf("Memory time = %+v, %v", storedItem.Snapshot(), err)
			}
			facts, err := memory.AppendLedger(t.Context(), agentmemory.FactBatch{
				Project: "/repo", SessionID: "session", Day: at.Format(time.DateOnly), Facts: []string{"fact"}, CapturedAt: at,
			})
			if err != nil || len(facts) != 1 {
				t.Fatalf("AppendLedger = %+v, %v", facts, err)
			}
			storedFacts, err := memory.PendingLedger(t.Context(), "/repo", 0, 1)
			if err != nil || len(storedFacts) != 1 || !storedFacts[0].CapturedAt.Equal(at) {
				t.Fatalf("Ledger time = %+v, %v", storedFacts, err)
			}
			published, err := memory.Reconcile(t.Context(), agentMemoryPublication(t, "/repo", agentmemory.State{}, facts[0].Sequence, nil, at))
			if err != nil || !published {
				t.Fatalf("Reconcile = %v, %v", published, err)
			}
			storedState, err := memory.State(t.Context(), "/repo")
			if err != nil || !storedState.UpdatedAt.Equal(at) {
				t.Fatalf("Curation time = %+v, %v", storedState, err)
			}
			draft := runDraft("run", "session")
			draft.CreatedAt = at
			if err := sqlite.NewRunStore(db).Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			models, tools := sqlite.NewModelInvocationStore(db), sqlite.NewToolInvocationStore(db)
			if err := models.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "model_call", at); err != nil {
				t.Fatal(err)
			}
			if err := models.CompleteModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "model_call", at, at, nil, nil); err != nil {
				t.Fatal(err)
			}
			attempts, err := models.PageModelInvocations(t.Context(), draft.RunID, 0, "", 1)
			if err != nil || len(attempts) != 1 || !attempts[0].StartedAt.Equal(at) || !attempts[0].FinishedAt.Equal(at) {
				t.Fatalf("Model attempt time = %+v, %v", attempts, err)
			}
			if err := tools.CompleteToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "tool_call", "tool_item", at, at); err != nil {
				t.Fatal(err)
			}
			toolAttempts, err := tools.ListSession(t.Context(), draft.SessionID)
			if err != nil || len(toolAttempts) != 1 || !toolAttempts[0].StartedAt.Equal(at) || !toolAttempts[0].FinishedAt.Equal(at) {
				t.Fatalf("Tool attempt time = %+v, %v", toolAttempts, err)
			}
			reservations := sqlite.NewChildRunStartReservationStore(db)
			record := sqlite.ChildRunStartReservationRecord{MemberID: "member", SessionID: "session", Payload: []byte(`{"run":"child"}`), CreatedAt: at}
			if err := reservations.Reserve(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			if changed, err := reservations.Conclude(t.Context(), record, sqlite.ChildRunStartConclusionStarted); err != nil || !changed {
				t.Fatalf("Conclude exact reservation = %v, %v", changed, err)
			}
		})
	}
}

func TestExecutionRecordsRejectUnrepresentableTimesBeforeWriting(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).Add(-time.Nanosecond).UTC(), time.Unix(0, math.MaxInt64).Add(time.Nanosecond).UTC()} {
		t.Run(at.Format(time.RFC3339Nano), func(t *testing.T) {
			db, err := sqlite.Open(t.Context(), ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			draft := runDraft("run", "session")
			if err := sqlite.NewRunStore(db).Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			if err := sqlite.NewModelInvocationStore(db).StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "model_call", at); err == nil {
				t.Error("Model attempt with unrepresentable start time was written")
			}
			if err := sqlite.NewToolInvocationStore(db).StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "tool_call", "tool_item", at); err == nil {
				t.Error("Tool attempt with unrepresentable start time was written")
			}
			if err := sqlite.NewChildRunStartReservationStore(db).Reserve(t.Context(), sqlite.ChildRunStartReservationRecord{
				MemberID: "member", SessionID: "session", Payload: []byte(`{"run":"child"}`), CreatedAt: at,
			}); err == nil {
				t.Error("Child reservation with unrepresentable creation time was written")
			}
			for _, table := range []string{"model_invocations", "tool_invocations", "child_run_start_reservations"} {
				var count int
				if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
					t.Errorf("%s rows = %d, %v; want no writes", table, count, err)
				}
			}
		})
	}
}
