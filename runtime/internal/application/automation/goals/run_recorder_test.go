package goals_test

import (
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestRunRecorderLetsTheCurrentGoalDecideATerminalRun(t *testing.T) {
	store := newMemStore()
	g, err := goal.New("s1", "do it", testGoalModelSelection(), run.Capabilities{}, "incarnation-recorded", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("new Goal: %v", err)
	}
	g = seedStoredGoal(t, store, g)
	recorder, err := goals.NewRunRecorder(store)
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
		current, found, err := storedGoal(t, store)
		if err != nil || !found || current.Version() != g.Version() || current.Status() != goal.StatusActive {
			t.Fatalf("Goal after %s = %+v found=%t err=%v, want it untouched", value.ID(), current.Snapshot(), found, err)
		}
	}

	if err := recorder.RecordRun(t.Context(), goalOwnedRun(owned, "run_failed", run.OutcomeFailed, nil, 1)); err != nil {
		t.Fatalf("RecordRun(failed): %v", err)
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
