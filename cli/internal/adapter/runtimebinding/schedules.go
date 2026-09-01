package runtimebinding

import (
	"context"
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
			if _, duplicate := seenIDs[value.ID]; duplicate {
				return nil, runtimeContractViolation("list schedules repeats %q", value.ID)
			}
			seenIDs[value.ID] = struct{}{}
			schedules = append(schedules, cloneSchedule(value))
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
	return scheduleResult("create schedule", "", created, err)
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
	return scheduleResult("update schedule", validated.ID, updated, err)
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
	if expectedID != "" && result.ID != expectedID {
		return protocol.Schedule{}, runtimeContractViolation("%s returned id %q for %q", operation, result.ID, expectedID)
	}
	return cloneSchedule(*result), nil
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

func cloneSchedule(value protocol.Schedule) protocol.Schedule {
	value.Workspace = cloneWorkspaceRef(value.Workspace)
	value.LastRunAt = clonePointer(value.LastRunAt)
	value.NextRunAt = clonePointer(value.NextRunAt)
	return value
}

func cloneWorkspaceRef(value *protocol.WorkspaceRef) *protocol.WorkspaceRef {
	if value == nil {
		return nil
	}
	return &protocol.WorkspaceRef{Path: value.Path}
}
