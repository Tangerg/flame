package schedules

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
)

// RunStarter is schedule firing's narrow view of the complete Run entry point.
type RunStarter interface {
	StartScheduled(ctx context.Context, cmd runs.ScheduledStart) (runs.StartResult, error)
}

// RunLauncher turns a due schedule into a headless application Run. It owns the
// schedule-specific defaults; the runs coordinator owns session creation,
// admission, execution, and lifecycle.
type RunLauncher struct {
	runs                 RunStarter
	defaultWorkspacePath string
}

// NewRunLauncher builds the scheduled-run execution strategy.
func NewRunLauncher(runStarter RunStarter, defaultWorkspacePath string) (RunLauncher, error) {
	if dependency.Missing(runStarter) {
		return RunLauncher{}, errors.New("schedules: run entry point is required")
	}
	return RunLauncher{runs: runStarter, defaultWorkspacePath: defaultWorkspacePath}, nil
}

// StartScheduledRun starts one schedule through the shared Run entry point, then
// immediately drops the unused event subscription.
func (r RunLauncher) StartScheduledRun(ctx context.Context, request schedule.RunRequest) error {
	command := runs.ScheduledStart{
		Request: request, DefaultWorkspacePath: r.defaultWorkspacePath,
	}
	startCtx, cancel := context.WithCancel(ctx)
	_, err := r.runs.StartScheduled(startCtx, command)
	cancel()
	return err
}
