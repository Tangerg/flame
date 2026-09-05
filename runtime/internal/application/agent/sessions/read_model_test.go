package sessions

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type activityRunStore struct {
	runs   []run.Run
	err    error
	onList func()
}

func (a activityRunStore) ListRuns(context.Context, string) ([]run.Run, error) {
	if a.onList != nil {
		a.onList()
	}
	return a.runs, a.err
}

func (a activityRunStore) ListNonTerminalRuns(context.Context) ([]run.Run, error) {
	if a.onList != nil {
		a.onList()
	}
	return a.runs, a.err
}

func TestActivitiesPreservesDurableRunReadFailure(t *testing.T) {
	want := errors.New("run store unavailable")
	coordinator := mustNewCoordinator(Dependencies{Runs: activityRunStore{err: want}})
	if _, err := coordinator.Activities(t.Context(), []string{"ses_1", "ses_2"}); !errors.Is(err, want) {
		t.Fatalf("Activities error = %v, want Run read failure", err)
	}
}

func TestActivitiesComeFromDurableRunLifecycle(t *testing.T) {
	coordinator := mustNewCoordinator(Dependencies{Runs: activityRunStore{runs: []run.Run{
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_running", SessionID: "ses_running", State: run.Running}),
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_waiting", SessionID: "ses_waiting", State: run.Waiting}),
	}}})
	activities, err := coordinator.Activities(
		t.Context(),
		[]string{"ses_running", "ses_waiting", "ses_idle"},
	)
	if err != nil {
		t.Fatalf("Activities: %v", err)
	}
	if activities["ses_running"] != ActivityRunning ||
		activities["ses_waiting"] != ActivityWaiting ||
		activities["ses_idle"] != ActivityIdle {
		t.Fatalf("Activities = %+v, want durable running/waiting/idle", activities)
	}
}
