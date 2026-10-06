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
	cmd      runs.StartCommand
	canceled chan struct{}
}

func (f *fakeRunStarter) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	f.cmd = cmd
	context.AfterFunc(ctx, func() { close(f.canceled) })
	return runs.StartResult{SessionID: cmd.Schedule.Request.SessionID(), RunID: cmd.Schedule.Request.RunID()}, nil
}

func TestRunLauncherUsesApplicationRunEntry(t *testing.T) {
	runStarter := &fakeRunStarter{canceled: make(chan struct{})}
	launcher := NewRunLauncher(runStarter, "/default")
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
	if runStarter.cmd.Schedule == nil || runStarter.cmd.Schedule.WorkspacePath != "/default" ||
		runStarter.cmd.Schedule.Request != request || runStarter.cmd.SessionID != "" {
		t.Fatalf("scheduled start = %+v", runStarter.cmd)
	}
	if len(runStarter.cmd.Input) != 1 || runStarter.cmd.Input[0].Text != "summarize" || runStarter.cmd.ModelSelection.Provider() != "p" || runStarter.cmd.ModelSelection.Model() != "m" {
		t.Fatalf("command mapping = %+v", runStarter.cmd)
	}
	<-runStarter.canceled
}
