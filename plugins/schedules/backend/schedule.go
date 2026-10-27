package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	scheduleapp "github.com/Tangerg/flame/plugins/schedules/internal/application"
	scheduledomain "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"
	"github.com/Tangerg/flame/qimu/portable"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/samber/lo"
)

type createScheduleArgs struct {
	Title           string `json:"title,omitempty" jsonschema_description:"Optional concise name for this recurring automation."`
	Instructions    string `json:"instructions" jsonschema:"minLength=1" jsonschema_description:"Complete self-contained instructions for each scheduled Agent Run."`
	WorkspacePath   string `json:"workspace_path,omitempty" jsonschema_description:"Workspace path for each scheduled Run. Omit to use the configured default workspace."`
	Provider        string `json:"provider,omitempty" jsonschema_description:"Model provider id. Set together with model only when the user explicitly chose both; otherwise omit both."`
	Model           string `json:"model,omitempty" jsonschema_description:"Model id. Set together with provider only when the user explicitly chose both; otherwise omit both."`
	ReasoningEffort string `json:"reasoning_effort,omitempty" jsonschema_description:"Optional reasoning intensity supported by the selected model. Omit to use the provider default."`
	Cron            string `json:"cron" jsonschema:"minLength=1" jsonschema_description:"Five-field cron (minute hour day-of-month month day-of-week), evaluated in UTC unless prefixed with CRON_TZ=Area/City, for example CRON_TZ=Asia/Shanghai 0 9 * * *."`
}

type updateScheduleArgs struct {
	ScheduleID       string  `json:"schedule_id" jsonschema:"minLength=1"`
	ExpectedRevision uint64  `json:"expected_revision" jsonschema:"minimum=1" jsonschema_description:"The revision returned by list_schedules, create_schedule or update_schedule. Refresh after a revision conflict."`
	Title            *string `json:"title,omitzero"`
	Instructions     *string `json:"instructions,omitzero"`
	WorkspacePath    *string `json:"workspace_path,omitzero"`
	Provider         *string `json:"provider,omitzero" jsonschema_description:"Set with model only when the user chooses an exact pair; set both to empty strings to restore Runtime defaults."`
	Model            *string `json:"model,omitzero"`
	ReasoningEffort  *string `json:"reasoning_effort,omitzero"`
	Cron             *string `json:"cron,omitzero" jsonschema_description:"Five-field cron evaluated in UTC unless prefixed with CRON_TZ=Area/City."`
	Enabled          *bool   `json:"enabled,omitzero" jsonschema_description:"False pauses future cron occurrences; already captured or accepted work is retained."`
}

type deleteScheduleArgs struct {
	ScheduleID string `json:"schedule_id" jsonschema:"minLength=1" jsonschema_description:"Exact id returned by list_schedules or create_schedule."`
}

type listScheduleArgs struct {
	Cursor string `json:"cursor,omitempty" jsonschema_description:"Opaque continuation returned by a previous list_schedules call. Omit for the first page."`
}

type scheduleListResponse struct {
	Schedules  []scheduleView `json:"schedules"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type scheduleResponse struct {
	Schedule scheduleView `json:"schedule"`
}

type scheduleDeleteResponse struct {
	ScheduleID string `json:"schedule_id"`
}

type scheduleView struct {
	ScheduleID      string `json:"schedule_id"`
	Revision        uint64 `json:"revision"`
	Title           string `json:"title,omitempty"`
	Instructions    string `json:"instructions"`
	WorkspacePath   string `json:"workspace_path,omitempty"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Cron            string `json:"cron"`
	Enabled         bool   `json:"enabled"`
	LastRunAt       string `json:"last_run_at,omitempty"`
	NextRunAt       string `json:"next_run_at,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
}

type scheduleManagement interface {
	ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[scheduledomain.Schedule], error)
	Create(ctx context.Context, draft scheduledomain.Draft) (scheduledomain.Schedule, error)
	Update(ctx context.Context, command scheduleapp.UpdateCommand) (scheduledomain.Schedule, error)
	Delete(ctx context.Context, id string) error
}

type scheduleManagementTools struct {
	coordinator scheduleManagement
	manual      func(context.Context, wire.ItemInput) (scheduleapp.RunReference, error)
}

func buildSchedules(coordinator scheduleManagement, manual func(context.Context, wire.ItemInput) (scheduleapp.RunReference, error)) ([]toolcontract.Tool, error) {
	if lo.IsNil(coordinator) {
		return nil, errors.New("schedules: tool coordinator is required")
	}
	if manual == nil {
		return nil, errors.New("schedules: manual Run operation is required")
	}
	t := &scheduleManagementTools{coordinator: coordinator, manual: manual}
	list, err := toolcontract.NewFunc[listScheduleArgs, scheduleListResponse](
		toolcontract.FuncConfig{
			Name:        "list_schedules",
			Description: "List recurring Agent Run schedules and their ids, revisions, instructions, cron expressions, model choices, and next-run state. Cron uses UTC unless it has a CRON_TZ=Area/City prefix. Read the current revision before editing.",
		},
		t.list,
	)
	if err != nil {
		return nil, err
	}
	create, err := toolcontract.NewFunc[createScheduleArgs, scheduleResponse](
		toolcontract.FuncConfig{
			Name: createScheduleTool,
			Description: "Create an enabled recurring schedule that starts a new Agent Run from self-contained instructions at each five-field cron occurrence. " +
				"Use only when the user explicitly asks for recurring automated work; do not use for the current request, a one-off future action, or an autonomous Goal.",
		},
		t.create,
	)
	if err != nil {
		return nil, err
	}
	update, err := toolcontract.NewFunc[updateScheduleArgs, scheduleResponse](
		toolcontract.FuncConfig{Name: updateScheduleTool, Description: "Edit or pause/resume an existing recurring schedule using its observed revision. Omitted fields stay unchanged; supplied empty title/workspace clears them. Retains the schedule identity and captured work. Use only for changes the user requests."},
		t.update,
	)
	if err != nil {
		return nil, err
	}
	deleteSchedule, err := toolcontract.NewFunc[deleteScheduleArgs, scheduleDeleteResponse](
		toolcontract.FuncConfig{
			Name:        deleteScheduleTool,
			Description: "Permanently delete one recurring Agent Run schedule by its exact schedule_id when the user requests removal. Captured and accepted work is retained. Use update_schedule to edit or pause a definition.",
		},
		t.delete,
	)
	if err != nil {
		return nil, err
	}
	manualRun, err := toolcontract.NewFunc[wire.ItemInput, scheduleapp.RunReference](
		toolcontract.FuncConfig{Name: runScheduleTool, Description: "Immediately run an existing schedule once, only when the user asks. Set ID to the exact schedule_id from list_schedules. Captures its current definition through the same durable manual Run operation; returned SessionID and RunID identify accepted work, not completed execution."},
		t.run,
	)
	if err != nil {
		return nil, err
	}
	return []toolcontract.Tool{list, create, update, deleteSchedule, manualRun}, nil
}

func (s *scheduleManagementTools) list(ctx context.Context, in listScheduleArgs) (scheduleListResponse, error) {
	page, err := s.coordinator.ListPage(ctx, in.Cursor, pagination.DefaultLimit())
	if err != nil {
		return scheduleListResponse{}, pluginsdk.DefiniteToolFailure(fmt.Errorf("list_schedules: %w", err))
	}
	views := make([]scheduleView, len(page.Rows))
	for i, sc := range page.Rows {
		views[i] = viewSchedule(sc)
	}
	return scheduleListResponse{Schedules: views, NextCursor: page.NextCursor}, nil
}

func (s *scheduleManagementTools) create(ctx context.Context, in createScheduleArgs) (scheduleResponse, error) {
	draft, err := in.draft()
	if err != nil {
		return scheduleResponse{}, pluginsdk.DefiniteToolFailure(err)
	}
	created, err := s.coordinator.Create(ctx, draft)
	if err != nil {
		return scheduleResponse{}, commandFailure(createScheduleTool, err)
	}
	return scheduleResponse{Schedule: viewSchedule(created)}, nil
}

func (in createScheduleArgs) draft() (scheduledomain.Draft, error) {
	selection, err := modelchoice.NewWithReasoningEffort(in.Provider, in.Model, in.ReasoningEffort)
	if err != nil {
		return scheduledomain.Draft{}, fmt.Errorf("create_schedule: %w", err)
	}
	return scheduledomain.Draft{
		Title:          in.Title,
		Instructions:   in.Instructions,
		CWD:            in.WorkspacePath,
		ModelSelection: selection,
		Cron:           in.Cron,
		Enabled:        true,
	}, nil
}

func (s *scheduleManagementTools) update(ctx context.Context, in updateScheduleArgs) (scheduleResponse, error) {
	value, err := s.coordinator.Update(ctx, scheduleapp.UpdateCommand{ID: in.ScheduleID, ExpectedRevision: in.ExpectedRevision, Patch: scheduleapp.Patch{Title: in.Title, Instructions: in.Instructions, CWD: in.WorkspacePath, Cron: in.Cron, Enabled: in.Enabled, ModelSelection: modelchoice.Patch{Provider: in.Provider, Model: in.Model, ReasoningEffort: in.ReasoningEffort}}})
	if err != nil {
		return scheduleResponse{}, commandFailure(updateScheduleTool, err)
	}
	return scheduleResponse{Schedule: viewSchedule(value)}, nil
}

func (s *scheduleManagementTools) run(ctx context.Context, in wire.ItemInput) (scheduleapp.RunReference, error) {
	value, err := s.manual(ctx, in)
	return value, commandFailure(runScheduleTool, err)
}

func (s *scheduleManagementTools) delete(ctx context.Context, in deleteScheduleArgs) (scheduleDeleteResponse, error) {
	if err := s.coordinator.Delete(ctx, in.ScheduleID); err != nil {
		return scheduleDeleteResponse{}, commandFailure(deleteScheduleTool, err)
	}
	return scheduleDeleteResponse(in), nil
}

const (
	createScheduleTool = "create_schedule"
	updateScheduleTool = "update_schedule"
	deleteScheduleTool = "delete_schedule"
	runScheduleTool    = "run_schedule"
)

// Command refusals are definite; unknown storage or acceptance failures are not.
func commandFailure(name string, cause error) error {
	if cause == nil {
		return nil
	}
	err := fmt.Errorf("%s: %w", name, cause)
	_, receipted := errors.AsType[*portable.Refusal](err)
	if errors.Is(err, scheduleapp.ErrRefused) || receipted || classify(cause) != "" {
		return pluginsdk.DefiniteToolFailure(err)
	}
	return err
}

func viewSchedule(sc scheduledomain.Schedule) scheduleView {
	return scheduleView{
		ScheduleID:      sc.ID(),
		Revision:        sc.Revision(),
		Title:           sc.Title(),
		Instructions:    sc.Instructions(),
		WorkspacePath:   sc.CWD(),
		Provider:        sc.ModelSelection().Provider(),
		Model:           sc.ModelSelection().Model(),
		ReasoningEffort: sc.ModelSelection().ReasoningEffort(),
		Cron:            sc.Cron(),
		Enabled:         sc.Enabled(),
		LastRunAt:       formatScheduleTime(sc.LastRunAt()),
		NextRunAt:       formatScheduleTime(sc.NextRunAt()),
		CreatedAt:       formatScheduleTime(sc.CreatedAt()),
	}
}

func formatScheduleTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
