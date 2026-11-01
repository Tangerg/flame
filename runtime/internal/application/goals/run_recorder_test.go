package goals_test

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/testsupport/goaltest"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestRunRecorderLetsTheCurrentGoalDecideATerminalRun(t *testing.T) {
	store := newMemStore()
	g, err := goaltest.New("s1", "do it", testGoalModelSelection(), run.Capabilities{}, "incarnation-recorded", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("new Goal: %v", err)
	}
	g = seedStoredGoal(t, store, g)
	evidence := &recordingEvidence{store: store}
	recorder, err := goals.NewRunRecorder(evidence)
	if err != nil {
		t.Fatalf("NewRunRecorder: %v", err)
	}
	owned := runs.StartCommand{SessionID: "s1", GoalIncarnationID: g.IncarnationID()}
	foreign := runs.StartCommand{SessionID: "s1", GoalIncarnationID: "incarnation-former"}

	for _, value := range []run.Run{
		goalOwnedRun(owned, "run_completed", run.OutcomeCompleted, nil, 1),
		goalOwnedRun(foreign, "run_former", run.OutcomeFailed, nil, 1),
	} {
		if err := recorder.RecordRun(t.Context(), value); err != nil {
			t.Fatalf("RecordRun(%s): %v", value.ID(), err)
		}
		if err := recorder.Drain(t.Context(), store, goaltest.Workflow{}, "s1", nil); err != nil {
			t.Fatal(err)
		}
		current, found, err := storedGoal(t, store)
		if err != nil || !found || current.Version() != g.Version() || current.Status() != goal.StatusActive {
			t.Fatalf("Goal after %s = %+v found=%t err=%v, want it untouched", value.ID(), current.Snapshot(), found, err)
		}
	}

	if err := recorder.RecordRun(t.Context(), goalOwnedRun(owned, "run_failed", run.OutcomeFailed, nil, 1)); err != nil {
		t.Fatalf("RecordRun(failed): %v", err)
	}
	if err := recorder.Drain(t.Context(), store, goaltest.Workflow{}, "s1", nil); err != nil {
		t.Fatal(err)
	}
	current, found, err := storedGoal(t, store)
	if err != nil || !found || current.Status() != goal.StatusPaused || current.Reason().Code() != goal.ReasonRunNotCompleted {
		t.Fatalf("Goal after a failed Run = %+v found=%t err=%v, want paused", current.Snapshot(), found, err)
	}
}

func storedGoal(t *testing.T, store *memStore) (goal.Goal, bool, error) {
	t.Helper()
	current, err := store.Get(t.Context(), "s1")
	if err != nil {
		return goal.Goal{}, false, err
	}
	value, found := current.Goal()
	return value, found, nil
}

type recordingEvidence struct {
	store   *memStore
	pending *goal.RunReceipt
}

func (r *recordingEvidence) CaptureGoalRun(ctx context.Context, value run.Run) error {
	current, err := r.store.Get(ctx, value.SessionID())
	if err != nil {
		return err
	}
	g, found := current.Goal()
	if !found {
		return nil
	}
	outcome, _ := value.Outcome()
	r.pending = &goal.RunReceipt{RunID: value.ID(), SessionID: value.SessionID(), IncarnationID: value.GoalIncarnationID(), Revision: g.Revision(), FinishedAt: value.FinishedAt(), Outcome: outcome}
	return nil
}
func (r *recordingEvidence) PendingGoalRun(ctx context.Context, sessionID string) (goal.RunReceipt, bool, error) {
	if r.pending == nil {
		return goal.RunReceipt{}, false, nil
	}
	return *r.pending, true, nil
}
func (r *recordingEvidence) SettleGoalRun(ctx context.Context, _ goal.RunReceipt, change *goal.Replacement) error {
	r.pending = nil
	if change != nil {
		_, err := r.store.Save(ctx, *change)
		return err
	}
	return nil
}
