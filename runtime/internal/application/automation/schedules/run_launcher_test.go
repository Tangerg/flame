package schedules

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

type fakeRunStarter struct {
	cmd      runs.ScheduledStart
	canceled chan struct{}
}

func (f *fakeRunStarter) StartScheduled(ctx context.Context, cmd runs.ScheduledStart) (runs.StartResult, error) {
	f.cmd = cmd
	context.AfterFunc(ctx, func() { close(f.canceled) })
	return runs.StartResult{SessionID: cmd.Request.SessionID(), RunID: cmd.Request.RunID()}, nil
}

func TestRunLauncherUsesApplicationRunEntry(t *testing.T) {
	runStarter := &fakeRunStarter{canceled: make(chan struct{})}
	launcher, err := NewRunLauncher(runStarter, "/default")
	if err != nil {
		t.Fatal(err)
	}
	scheduled := mustStoredSchedule(t, schedule.Snapshot{
		ID: "sch_1", Instructions: "summarize", ModelSelection: testsupport.MustModelSelection("p", "m"),
	})

	ranAt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	request, err := schedule.ManualRunRequest(scheduled, "ses_manual", "run_manual", ranAt)
	if err != nil {
		t.Fatalf("ManualRunRequest: %v", err)
	}
	if err := launcher.StartScheduledRun(context.Background(), request); err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	if runStarter.cmd.DefaultWorkspacePath != "/default" || runStarter.cmd.Request != request {
		t.Fatalf("scheduled start = %+v", runStarter.cmd)
	}
	<-runStarter.canceled
}

func TestRunLauncherRequiresRunEntry(t *testing.T) {
	var typedNil *fakeRunStarter
	for _, entry := range []RunStarter{nil, typedNil} {
		if _, err := NewRunLauncher(entry, "/default"); err == nil {
			t.Fatal("launcher accepted a missing Run entry point")
		}
	}
}
