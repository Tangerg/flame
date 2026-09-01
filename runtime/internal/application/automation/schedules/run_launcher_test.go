package schedules

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type fakeRunStarter struct {
	cmd      runs.StartCommand
	canceled chan struct{}
}

func (f *fakeRunStarter) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	f.cmd = cmd
	context.AfterFunc(ctx, func() { close(f.canceled) })
	return runs.StartResult{SessionID: "ses_scheduled", RunID: "run_scheduled"}, nil
}

func TestRunLauncherUsesApplicationRunEntry(t *testing.T) {
	runStarter := &fakeRunStarter{canceled: make(chan struct{})}
	launcher := NewRunLauncher(runStarter, "/default")
	scheduled := mustStoredSchedule(t, schedule.Snapshot{
		ID: "sch_1", Instructions: "summarize", ModelSelection: mustScheduleSelection("p", "m"),
	})

	ranAt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	request, err := schedule.ManualRunRequest(scheduled, "ses_manual", "run_manual", ranAt)
	if err != nil {
		t.Fatalf("ManualRunRequest: %v", err)
	}
	startedRun, err := launcher.StartScheduledRun(context.Background(), request)
	if err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	if startedRun.SessionID != "ses_scheduled" || startedRun.RunID != "run_scheduled" {
		t.Fatalf("started Run=%+v", startedRun)
	}
	if runStarter.cmd.DefaultWorkspacePath != "/default" || runStarter.cmd.NewSessionTitle != "" {
		t.Fatalf("command defaults = %+v", runStarter.cmd)
	}
	if runStarter.cmd.NewSessionID != "ses_manual" || runStarter.cmd.RunID != "run_manual" || runStarter.cmd.ScheduleFiring != "" {
		t.Fatalf("manual schedule identities = %+v", runStarter.cmd)
	}
	if len(runStarter.cmd.Input) != 1 || runStarter.cmd.Input[0].Text != "summarize" || runStarter.cmd.ModelSelection.Provider() != "p" || runStarter.cmd.ModelSelection.Model() != "m" {
		t.Fatalf("command mapping = %+v", runStarter.cmd)
	}
	if runStarter.cmd.ManualScheduleRun == nil || runStarter.cmd.ManualScheduleRun.ScheduleID() != "sch_1" ||
		!runStarter.cmd.ManualScheduleRun.RanAt().Equal(ranAt) {
		t.Fatalf("manual schedule Run fact = %+v", runStarter.cmd.ManualScheduleRun)
	}
	<-runStarter.canceled
}

func mustScheduleSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}
