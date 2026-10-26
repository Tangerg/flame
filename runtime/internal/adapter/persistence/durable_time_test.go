package persistence

import (
	"math"
	"testing"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

func TestDurableOwnerTimesRoundTripAtExactBounds(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).UTC(), time.Unix(0, 0).UTC(), time.Unix(0, math.MaxInt64).UTC()} {
		t.Run(at.Format(time.RFC3339Nano), func(t *testing.T) {
			db, err := openDatabase(t.Context(), ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			goals, plans := newGoalStore(db), newPlanStore(db)
			seedSession(t, newSessionStore(db), "session")
			value, err := goal.New("session", "objective", modelchoice.Selection{}, run.Capabilities{}, "incarnation", at)
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
			draft := runDraft("run", "session")
			draft.CreatedAt = at
			if err := newRunStore(db).Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			models, tools := newModelInvocationStore(db), newToolInvocationStore(db)
			if err := models.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "model_call", at, run.ModelConversation); err != nil {
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
			reservations := newChildRunStartReservationStore(db)
			record := ChildRunStartReservationRecord{MemberID: "member", SessionID: "session", Payload: []byte(`{"run":"child"}`), CreatedAt: at}
			if err := reservations.Reserve(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			if changed, err := reservations.Conclude(t.Context(), record, ChildRunStartConclusionStarted); err != nil || !changed {
				t.Fatalf("Conclude exact reservation = %v, %v", changed, err)
			}
		})
	}
}

func TestExecutionRecordsRejectUnrepresentableTimesBeforeWriting(t *testing.T) {
	for _, at := range []time.Time{time.Unix(0, math.MinInt64).Add(-time.Nanosecond).UTC(), time.Unix(0, math.MaxInt64).Add(time.Nanosecond).UTC()} {
		t.Run(at.Format(time.RFC3339Nano), func(t *testing.T) {
			db, err := openDatabase(t.Context(), ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			draft := runDraft("run", "session")
			if err := newRunStore(db).Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			if err := newModelInvocationStore(db).StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "model_call", at, run.ModelConversation); err == nil {
				t.Error("Model attempt with unrepresentable start time was written")
			}
			if err := newToolInvocationStore(db).StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "tool_call", "tool_item", at); err == nil {
				t.Error("Tool attempt with unrepresentable start time was written")
			}
			if err := newChildRunStartReservationStore(db).Reserve(t.Context(), ChildRunStartReservationRecord{
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
