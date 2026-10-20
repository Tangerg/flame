package tools

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/plugin-sdk/go/toolfailure"
	scheduleapp "github.com/Tangerg/flame/plugins/schedules/backend/application"
	scheduledomain "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/dependency"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/flame/runtime/pagination"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type createScheduleArgs struct {
	Title           string `json:"title,omitempty" jsonschema_description:"Optional concise name for this recurring automation."`
	Instructions    string `json:"instructions" jsonschema:"minLength=1" jsonschema_description:"Complete self-contained instructions for each scheduled Agent Run."`
	WorkspacePath   string `json:"workspace_path,omitempty" jsonschema_description:"Workspace path for each scheduled Run. Omit to use the configured default workspace."`
	Provider        string `json:"provider,omitempty" jsonschema_description:"Model provider id. Set together with model only when the user explicitly chose both; otherwise omit both."`
	Model           string `json:"model,omitempty" jsonschema_description:"Model id. Set together with provider only when the user explicitly chose both; otherwise omit both."`
	ReasoningEffort string `json:"reasoning_effort,omitempty" jsonschema_description:"Optional reasoning intensity supported by the selected model. Omit to use the provider default."`
	Cron            string `json:"cron" jsonschema:"minLength=1" jsonschema_description:"Five-field cron expression: minute hour day-of-month month day-of-week."`
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

type ScheduleManagement interface {
	ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[scheduledomain.Schedule], error)
	Create(ctx context.Context, draft scheduledomain.Draft) (scheduledomain.Schedule, error)
	Delete(ctx context.Context, id string) error
}

type scheduleManagementTools struct{ coordinator ScheduleManagement }

func BuildSchedules(coordinator ScheduleManagement) ([]toolcontract.Tool, error) {
	if dependency.Missing(coordinator) {
		return nil, errors.New("schedules: tool coordinator is required")
	}
	t := &scheduleManagementTools{coordinator: coordinator}
	list, err := toolcontract.NewFunc[listScheduleArgs, scheduleListResponse](
		toolcontract.FuncConfig{
			Name:        "list_schedules",
			Description: "List recurring Agent Run schedules and their ids, instructions, cron expressions, model choices, and next-run state. Use this before deleting or replacing a schedule when its exact id is unknown.",
		},
		t.list,
	)
	if err != nil {
		return nil, err
	}
	create, err := toolcontract.NewFunc[createScheduleArgs, scheduleResponse](
		toolcontract.FuncConfig{
			Name: "create_schedule",
			Description: "Create an enabled recurring schedule that starts a new Agent Run from self-contained instructions at each five-field cron occurrence. " +
				"Use only when the user explicitly asks for recurring automated work; do not use for the current request, a one-off future action, or an autonomous Goal.",
		},
		t.create,
	)
	if err != nil {
		return nil, err
	}
	deleteSchedule, err := toolcontract.NewFunc[deleteScheduleArgs, scheduleDeleteResponse](
		toolcontract.FuncConfig{
			Name:        "delete_schedule",
			Description: "Permanently delete one recurring Agent Run schedule by its exact schedule_id. Use list_schedules first when the id is uncertain. To change a schedule, delete it and create the replacement explicitly.",
		},
		t.delete,
	)
	if err != nil {
		return nil, err
	}
	return []toolcontract.Tool{list, create, deleteSchedule}, nil
}

func (s *scheduleManagementTools) list(ctx context.Context, in listScheduleArgs) (scheduleListResponse, error) {
	page, err := s.coordinator.ListPage(ctx, in.Cursor, pagination.DefaultLimit())
	if err != nil {
		return scheduleListResponse{}, toolfailure.Definite(fmt.Errorf("list_schedules: %w", err))
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
		return scheduleResponse{}, toolfailure.Definite(err)
	}
	created, err := s.coordinator.Create(ctx, draft)
	if err != nil {
		return scheduleResponse{}, CommandFailure("create_schedule", err)
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

// CreateOutput shares the normal Tool's output codec with read-only recovery.
func CreateOutput(created scheduledomain.Schedule) (chat.ToolOutput, error) {
	encoded, err := json.Marshal(scheduleResponse{Schedule: viewSchedule(created)})
	if err != nil {
		return chat.ToolOutput{}, err
	}
	return chat.NewJSONToolOutput(encoded)
}

func (s *scheduleManagementTools) delete(ctx context.Context, in deleteScheduleArgs) (scheduleDeleteResponse, error) {
	if err := s.coordinator.Delete(ctx, in.ScheduleID); err != nil {
		return scheduleDeleteResponse{}, CommandFailure("delete_schedule", err)
	}
	return scheduleDeleteResponse(in), nil
}

// CommandFailure preserves receipted refusals while storage failures stay unknown.
func CommandFailure(name string, cause error) error {
	err := fmt.Errorf("%s: %w", name, cause)
	_, receipted := errors.AsType[*capability.Refusal](err)
	if errors.Is(err, scheduleapp.ErrRefused) || receipted {
		return toolfailure.Definite(err)
	}
	return err
}

func viewSchedule(sc scheduledomain.Schedule) scheduleView {
	return scheduleView{
		ScheduleID:      sc.ID(),
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
