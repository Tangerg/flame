package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	scheduleapp "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/protocol"
)

// Schedule operations manage the cron-triggered headless runs the
// backend worker fires. A schedule stores the final instructions
// text, so the runtime fires it without resolving a template.

// UpdateSchedule applies a revision-guarded partial patch. The schedule use case
// recomputes due time when cron or enabled changes and clears it when disabled.
func (s *Handler) UpdateSchedule(ctx context.Context, in protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
	updated, err := s.schedules.Update(ctx, scheduleapp.UpdateCommand{
		ID:               in.ID,
		ExpectedRevision: in.ExpectedRevision,
		Patch: scheduleapp.Patch{
			Title:        in.Title,
			Instructions: in.Instructions,
			CWD:          scheduleWorkspacePathPatch(in.Workspace, in.WorkspaceMode),
			ModelSelection: modelref.Patch{
				Provider: in.Provider, Model: in.Model, ReasoningEffort: in.ReasoningEffort,
			},
			Cron:    in.Cron,
			Enabled: in.Enabled,
		},
	})
	if err != nil {
		return nil, mapScheduleErr(err, in.ID)
	}
	wire := presentSchedule(updated)
	return &wire, nil
}

func scheduleWorkspacePathPatch(ref *protocol.WorkspaceRef, mode protocol.ScheduleWorkspaceMode) *string {
	if mode == protocol.ScheduleWorkspaceDefault {
		path := ""
		return &path
	}
	return workspacePathPatch(ref)
}

func mapScheduleErr(err error, id string) error {
	if errors.Is(err, plugin.ErrCommandUnconfirmed) {
		return err
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, plugin.ErrUnavailable) {
		return errors.Join(protocol.ErrPluginUnavailable, err)
	}
	if errors.Is(err, schedule.ErrNotFound) {
		return NewFailure(errors.Join(protocol.ErrScheduleNotFound, err), fmt.Sprintf("schedule %q no longer exists; refresh the schedule list", id))
	}
	// Workspace failures have one translation. A schedule's cwd can produce any
	// of them, not just an unavailable one, and wireWorkspaceError returns
	// anything outside that vocabulary unchanged.
	if wired := wireWorkspaceError(err); wired != err {
		return wired
	}
	if errors.Is(err, schedule.ErrRevisionConflict) {
		return NewFailure(errors.Join(protocol.ErrRevisionConflict, err), fmt.Sprintf("schedule %q changed after it was read", id))
	}
	if errors.Is(err, schedule.ErrIDRequired) ||
		errors.Is(err, schedule.ErrRevisionRequired) ||
		errors.Is(err, schedule.ErrInstructionsRequired) ||
		errors.Is(err, schedule.ErrCronRequired) ||
		modelref.IsInvalid(err) ||
		errors.Is(err, modelref.ErrUnsupported) ||
		errors.Is(err, schedule.ErrInvalidCron) {
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	}
	return err
}

// presentSchedule maps a domain schedule to its protocol shape, projecting the zero
// time (never fired / unscheduled) to an omitted field rather than a fake epoch.
func presentSchedule(scheduled schedule.Schedule) protocol.Schedule {
	presented := protocol.Schedule{
		ID:              scheduled.ID(),
		Title:           scheduled.Title(),
		Instructions:    scheduled.Instructions(),
		Workspace:       workspaceRefFromPath(scheduled.CWD()),
		Provider:        scheduled.ModelSelection().Provider(),
		Model:           scheduled.ModelSelection().Model(),
		ReasoningEffort: scheduled.ModelSelection().ReasoningEffort(),
		Cron:            scheduled.Cron(),
		Enabled:         scheduled.Enabled(),
		CreatedAt:       scheduled.CreatedAt(),
		Revision:        scheduled.Revision(),
	}
	if !scheduled.LastRunAt().IsZero() {
		lastRunAt := scheduled.LastRunAt()
		presented.LastRunAt = &lastRunAt
	}
	if !scheduled.NextRunAt().IsZero() {
		nextRunAt := scheduled.NextRunAt()
		presented.NextRunAt = &nextRunAt
	}
	return presented
}
