package runtimebinding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

const (
	schedulePageLimit = 100
	// maximumSchedulePageRequests bounds full-catalog materialization to at
	// most 10,000 schedules when the Runtime serves the requested page width.
	// The request bound remains authoritative for a Runtime that under-fills.
	maximumSchedulePageRequests = 100
)

type scheduleBinding interface {
	ListSchedules(context.Context, protocol.PageQuery, flameruntime.CallOptions) (*protocol.Page[protocol.Schedule], error)
	CreateSchedule(context.Context, protocol.CreateScheduleRequest, flameruntime.CommandOptions) (*protocol.Schedule, error)
	UpdateSchedule(context.Context, protocol.UpdateScheduleRequest, flameruntime.CommandOptions) (*protocol.Schedule, error)
	DeleteSchedule(context.Context, protocol.DeleteScheduleRequest, flameruntime.CommandOptions) error
	RunScheduleNow(context.Context, protocol.RunScheduleNowRequest, flameruntime.CommandOptions) (*protocol.RunScheduleNowResponse, error)
}

func (r *Connection) Schedules(ctx context.Context) ([]protocol.Schedule, error) {
	var schedules []protocol.Schedule
	seenIDs := make(map[string]struct{})
	cursors, err := newCursorTraversal("list schedules", "", maximumSchedulePageRequests)
	if err != nil {
		return nil, err
	}
	for {
		cursor := cursors.Current()
		page, err := r.schedules.ListSchedules(ctx, protocol.PageQuery{Cursor: cursor, Limit: protocolPositiveInt(schedulePageLimit)}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, runtimeContractViolation("list schedules returned a nil page")
		}
		if len(page.Data) > schedulePageLimit {
			return nil, runtimeContractViolation("list schedules returned %d rows for limit %d", len(page.Data), schedulePageLimit)
		}
		for index, value := range page.Data {
			if validateErr := protocol.ValidateWireTree(value); validateErr != nil {
				return nil, runtimeContractViolation("list schedules item %d after cursor %q is invalid: %v", index+1, cursor, validateErr)
			}
			if validateErr := validateScheduleProjection(value); validateErr != nil {
				return nil, runtimeContractViolation("list schedules item %d after cursor %q is invalid: %v", index+1, cursor, validateErr)
			}
			if _, duplicate := seenIDs[value.ID]; duplicate {
				return nil, runtimeContractViolation("list schedules repeats %q", value.ID)
			}
			if len(schedules) != 0 {
				previous := schedules[len(schedules)-1]
				if value.CreatedAt.After(previous.CreatedAt) ||
					(value.CreatedAt.Equal(previous.CreatedAt) && value.ID > previous.ID) {
					return nil, runtimeContractViolation(
						"list schedules returned schedule %q out of catalog order after %q", value.ID, previous.ID,
					)
				}
			}
			seenIDs[value.ID] = struct{}{}
			schedules = append(schedules, value)
		}
		more, err := cursors.Advance(page.NextCursor)
		if err != nil {
			return nil, err
		}
		if !more {
			return schedules, nil
		}
	}
}

func (r *Connection) Create(ctx context.Context, request protocol.CreateScheduleRequest) (protocol.Schedule, error) {
	validated := cloneCreateScheduleRequest(request)
	if err := protocol.ValidateWireTree(validated); err != nil {
		return protocol.Schedule{}, fmt.Errorf("create schedule: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.Schedule{}, err
	}
	if validated.Workspace != nil {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: validated.Workspace.Path})
		if resolveErr != nil {
			return protocol.Schedule{}, fmt.Errorf("create schedule workspace: %w", resolveErr)
		}
		validated.Workspace = &protocol.WorkspaceRef{Path: resolved.Path}
	}
	created, err := r.schedules.CreateSchedule(ctx, validated, options)
	result, err := scheduleResult("create schedule", "", created, err)
	if err != nil {
		return protocol.Schedule{}, err
	}
	if err := validateCreateScheduleResult(validated, result); err != nil {
		return protocol.Schedule{}, runtimeContractViolation("create schedule returned an invalid acknowledgement: %v", err)
	}
	return result, nil
}

func (r *Connection) Update(ctx context.Context, request protocol.UpdateScheduleRequest) (protocol.Schedule, error) {
	validated := cloneUpdateScheduleRequest(request)
	if err := protocol.ValidateWireTree(validated); err != nil {
		return protocol.Schedule{}, fmt.Errorf("update schedule: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.Schedule{}, err
	}
	if validated.Workspace != nil {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: validated.Workspace.Path})
		if resolveErr != nil {
			return protocol.Schedule{}, fmt.Errorf("update schedule workspace: %w", resolveErr)
		}
		validated.Workspace = &protocol.WorkspaceRef{Path: resolved.Path}
	}
	updated, err := r.schedules.UpdateSchedule(ctx, validated, options)
	result, err := scheduleResult("update schedule", validated.ID, updated, err)
	if err != nil {
		return protocol.Schedule{}, err
	}
	if err := validateUpdateScheduleResult(validated, result); err != nil {
		return protocol.Schedule{}, runtimeContractViolation("update schedule returned an invalid acknowledgement: %v", err)
	}
	return result, nil
}

func (r *Connection) Delete(ctx context.Context, id string) error {
	request := protocol.DeleteScheduleRequest{ID: strings.TrimSpace(id)}
	if err := protocol.ValidateWireTree(request); err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.schedules.DeleteSchedule(ctx, request, options))
}

func (r *Connection) RunNow(ctx context.Context, id string) (protocol.RunScheduleNowResponse, error) {
	request := protocol.RunScheduleNowRequest{ID: strings.TrimSpace(id)}
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.RunScheduleNowResponse{}, fmt.Errorf("run schedule now: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.RunScheduleNowResponse{}, err
	}
	result, err := r.schedules.RunScheduleNow(ctx, request, options)
	if err != nil {
		return protocol.RunScheduleNowResponse{}, classifyError(err)
	}
	if result == nil {
		return protocol.RunScheduleNowResponse{}, runtimeContractViolation("run schedule now returned nil")
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return protocol.RunScheduleNowResponse{}, runtimeContractViolation("run schedule now returned an invalid response: %v", err)
	}
	return *result, nil
}

func scheduleResult(operation, expectedID string, result *protocol.Schedule, err error) (protocol.Schedule, error) {
	if err != nil {
		return protocol.Schedule{}, classifyError(err)
	}
	if result == nil {
		return protocol.Schedule{}, runtimeContractViolation("%s returned nil", operation)
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return protocol.Schedule{}, runtimeContractViolation("%s returned an invalid schedule: %v", operation, err)
	}
	if err := validateScheduleProjection(*result); err != nil {
		return protocol.Schedule{}, runtimeContractViolation("%s returned an invalid schedule: %v", operation, err)
	}
	if expectedID != "" && result.ID != expectedID {
		return protocol.Schedule{}, runtimeContractViolation("%s returned id %q for %q", operation, result.ID, expectedID)
	}
	return *result, nil
}

func validateScheduleProjection(result protocol.Schedule) error {
	var problems []error
	if result.LastRunAt != nil {
		if result.LastRunAt.IsZero() {
			problems = append(problems, errors.New("last-run time is zero"))
		} else if result.LastRunAt.Before(result.CreatedAt) {
			problems = append(problems, errors.New("last run precedes creation"))
		}
	}
	if result.NextRunAt != nil && result.NextRunAt.IsZero() {
		problems = append(problems, errors.New("next-run time is zero"))
	}
	if result.Enabled != (result.NextRunAt != nil) {
		problems = append(problems, errors.New("enabled state and next-run cursor disagree"))
	}
	return errors.Join(problems...)
}

func validateCreateScheduleResult(request protocol.CreateScheduleRequest, result protocol.Schedule) error {
	var problems []error
	if result.Revision != 1 {
		problems = append(problems, fmt.Errorf("initial revision is %d, want 1", result.Revision))
	}
	if result.Title != request.Title {
		problems = append(problems, fmt.Errorf("title is %q, want %q", result.Title, request.Title))
	}
	if result.Instructions != request.Instructions {
		problems = append(problems, errors.New("instructions differ from the request"))
	}
	if !equalScheduleWorkspace(result.Workspace, request.Workspace) {
		problems = append(problems, errors.New("workspace differs from the request"))
	}
	if result.Provider != request.Provider || result.Model != request.Model || result.ReasoningEffort != request.ReasoningEffort {
		problems = append(problems, errors.New("model selection differs from the request"))
	}
	if result.Cron != request.Cron {
		problems = append(problems, fmt.Errorf("cron is %q, want %q", result.Cron, request.Cron))
	}
	if !result.Enabled {
		problems = append(problems, errors.New("new schedule is disabled"))
	}
	if result.LastRunAt != nil {
		problems = append(problems, errors.New("new schedule has already run"))
	}
	return errors.Join(problems...)
}

func validateUpdateScheduleResult(request protocol.UpdateScheduleRequest, result protocol.Schedule) error {
	var problems []error
	if result.Revision != request.ExpectedRevision+1 {
		problems = append(problems, fmt.Errorf("revision is %d, want %d", result.Revision, request.ExpectedRevision+1))
	}
	if request.Title != nil && result.Title != *request.Title {
		problems = append(problems, fmt.Errorf("title is %q, want %q", result.Title, *request.Title))
	}
	if request.Instructions != nil && result.Instructions != *request.Instructions {
		problems = append(problems, errors.New("instructions differ from the request"))
	}
	if request.Workspace != nil && !equalScheduleWorkspace(result.Workspace, request.Workspace) {
		problems = append(problems, errors.New("workspace differs from the request"))
	}
	if request.WorkspaceMode == protocol.ScheduleWorkspaceDefault && result.Workspace != nil {
		problems = append(problems, errors.New("default workspace update retained an explicit workspace"))
	}
	if request.Provider != nil {
		if result.Provider != *request.Provider || result.Model != *request.Model {
			problems = append(problems, errors.New("model identity differs from the request"))
		}
		expectedEffort := ""
		if request.ReasoningEffort != nil {
			expectedEffort = *request.ReasoningEffort
		}
		if result.ReasoningEffort != expectedEffort {
			problems = append(problems, fmt.Errorf("reasoning effort is %q, want %q", result.ReasoningEffort, expectedEffort))
		}
	} else if request.ReasoningEffort != nil && result.ReasoningEffort != *request.ReasoningEffort {
		problems = append(problems, fmt.Errorf("reasoning effort is %q, want %q", result.ReasoningEffort, *request.ReasoningEffort))
	}
	if request.Cron != nil && result.Cron != *request.Cron {
		problems = append(problems, fmt.Errorf("cron is %q, want %q", result.Cron, *request.Cron))
	}
	if request.Enabled != nil && result.Enabled != *request.Enabled {
		problems = append(problems, fmt.Errorf("enabled is %t, want %t", result.Enabled, *request.Enabled))
	}
	return errors.Join(problems...)
}

func equalScheduleWorkspace(left, right *protocol.WorkspaceRef) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Path == right.Path
}

func cloneCreateScheduleRequest(value protocol.CreateScheduleRequest) protocol.CreateScheduleRequest {
	value.Workspace = cloneWorkspaceRef(value.Workspace)
	return value
}

func cloneUpdateScheduleRequest(value protocol.UpdateScheduleRequest) protocol.UpdateScheduleRequest {
	value.Title = clonePointer(value.Title)
	value.Instructions = clonePointer(value.Instructions)
	value.Workspace = cloneWorkspaceRef(value.Workspace)
	value.Provider = clonePointer(value.Provider)
	value.Model = clonePointer(value.Model)
	value.ReasoningEffort = clonePointer(value.ReasoningEffort)
	value.Cron = clonePointer(value.Cron)
	value.Enabled = clonePointer(value.Enabled)
	return value
}

func cloneWorkspaceRef(value *protocol.WorkspaceRef) *protocol.WorkspaceRef {
	if value == nil {
		return nil
	}
	return &protocol.WorkspaceRef{Path: value.Path}
}
