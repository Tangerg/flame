package schedules

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// RunStarter is schedule firing's narrow view of the complete Run entry point.
type RunStarter interface {
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
}

// RunLauncher turns a due schedule into a headless application Run. It owns the
// schedule-specific defaults; the runs coordinator owns session creation,
// admission, execution, and lifecycle.
type RunLauncher struct {
	runs                 RunStarter
	defaultWorkspacePath string
}

// NewRunLauncher builds the scheduled-run execution strategy.
func NewRunLauncher(runStarter RunStarter, defaultWorkspacePath string) RunLauncher {
	return RunLauncher{runs: runStarter, defaultWorkspacePath: defaultWorkspacePath}
}

// StartScheduledRun starts one schedule through the shared Run entry point, then
// immediately drops the unused event subscription.
func (r RunLauncher) StartScheduledRun(ctx context.Context, request schedule.RunRequest) error {
	execution := request.Execution()
	workspacePath := execution.CWD()
	if workspacePath == "" {
		workspacePath = r.defaultWorkspacePath
	}
	command := runs.StartCommand{
		RunID:                request.RunID(),
		NewSessionID:         request.SessionID(),
		ScheduleFiring:       request.OccurrenceID(),
		DefaultWorkspacePath: workspacePath,
		NewSessionTitle:      execution.Title(),
		ModelSelection:       execution.ModelSelection(),
		Input:                []transcript.ContentBlock{{Kind: transcript.TextContent, Text: execution.Instructions()}},
	}
	if record, manual := request.ManualRecord(); manual {
		command.ManualScheduleRun = &record
	}
	startCtx, cancel := context.WithCancel(ctx)
	_, err := r.runs.Start(startCtx, command)
	cancel()
	return err
}
