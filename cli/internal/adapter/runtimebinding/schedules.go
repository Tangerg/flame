package runtimebinding

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
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
	CreateSchedule(context.Context, protocol.CreateScheduleRequest, flameruntime.CommandOptions) (*protocol.Schedule, error)
	UpdateSchedule(context.Context, protocol.UpdateScheduleRequest, flameruntime.CommandOptions) (*protocol.Schedule, error)
}

func (r *Connection) Schedules(ctx context.Context) ([]protocol.Schedule, error) {
	bound, err := r.selectedSchedules(ctx)
	if err != nil {
		return nil, err
	}
	var schedules []protocol.Schedule
	seenIDs := make(map[string]struct{})
	cursors, err := newCursorTraversal("list schedules", "", maximumSchedulePageRequests)
	if err != nil {
		return nil, err
	}
	for {
		cursor := cursors.Current()
		input, err := jsoninput.Encode(SchedulesListV1Input{Cursor: cursor, Limit: new(int64(schedulePageLimit))})
		if err != nil {
			return nil, err
		}
		request := bound
		request.Input = input.JSON()
		reply, err := r.CallPlugin(ctx, request, "")
		if err != nil {
			return nil, classifyError(err)
		}
		var page SchedulesListV1Output
		if err := json.Unmarshal(reply.Output, &page, json.RejectUnknownMembers(true)); err != nil {
			return nil, runtimeContractViolation("list schedules output: %v", err)
		}
		if len(page.Rows) > schedulePageLimit {
			return nil, runtimeContractViolation("list schedules returned %d rows for limit %d", len(page.Rows), schedulePageLimit)
		}
		for _, row := range page.Rows {
			if _, duplicate := seenIDs[row.ID]; duplicate {
				return nil, runtimeContractViolation("list schedules repeats %q", row.ID)
			}
			seenIDs[row.ID] = struct{}{}
			value := protocol.Schedule{ID: row.ID, Title: row.Title, Instructions: row.Instructions, Cron: row.Cron, Enabled: !row.NextRunAt.IsZero(), CreatedAt: row.CreatedAt, Revision: row.Revision, Provider: row.ModelSelection["provider"], Model: row.ModelSelection["model"], ReasoningEffort: row.ModelSelection["reasoningEffort"]}
			if row.CWD != "" {
				value.Workspace = &protocol.WorkspaceRef{Path: row.CWD}
			}
			if !row.NextRunAt.IsZero() {
				value.NextRunAt = &row.NextRunAt
			}
			if !row.LastRunAt.IsZero() {
				value.LastRunAt = &row.LastRunAt
			}
			if err := protocol.ValidateWireTree(value); err != nil {
				return nil, runtimeContractViolation("list schedules row: %v", err)
			}
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
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.Schedule{}, fmt.Errorf("create schedule: %w", err)
	}
	options := r.commandOptions()
	if request.Workspace != nil {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: request.Workspace.Path})
		if resolveErr != nil {
			return protocol.Schedule{}, fmt.Errorf("create schedule workspace: %w", resolveErr)
		}
		request.Workspace = &protocol.WorkspaceRef{Path: resolved.Path}
	}
	created, err := r.schedules.CreateSchedule(ctx, request, options)
	return scheduleResult("create schedule", "", created, err)
}

func (r *Connection) Update(ctx context.Context, request protocol.UpdateScheduleRequest) (protocol.Schedule, error) {
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.Schedule{}, fmt.Errorf("update schedule: %w", err)
	}
	options := r.commandOptions()
	if request.Workspace != nil {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: request.Workspace.Path})
		if resolveErr != nil {
			return protocol.Schedule{}, fmt.Errorf("update schedule workspace: %w", resolveErr)
		}
		request.Workspace = &protocol.WorkspaceRef{Path: resolved.Path}
	}
	updated, err := r.schedules.UpdateSchedule(ctx, request, options)
	return scheduleResult("update schedule", request.ID, updated, err)
}

func (r *Connection) PrepareDelete(ctx context.Context, id string) (protocol.CallPluginRequest, error) {
	input, err := jsoninput.Encode(SchedulesDeleteV1Input{ID: strings.TrimSpace(id)})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return r.prepareSchedulesCommand(ctx, schedulesDeleteV1Method, input)
}

func (r *Connection) PrepareRunNow(ctx context.Context, id string) (protocol.CallPluginRequest, error) {
	input, err := jsoninput.Encode(SchedulesRunNowV1Input{ID: strings.TrimSpace(id)})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return r.prepareSchedulesCommand(ctx, schedulesRunNowV1Method, input)
}

func (r *Connection) prepareSchedulesCommand(ctx context.Context, name string, input jsoninput.Input) (protocol.CallPluginRequest, error) {
	bound, err := r.selectedSchedules(ctx)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	contract, err := capability.Parse([]byte(bound.Contract))
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	method, found := contract.Method(name)
	if !found {
		return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
	}
	if err := method.ValidateInput(input); err != nil {
		return protocol.CallPluginRequest{}, fmt.Errorf("schedule command: %w", err)
	}
	bound.Method, bound.Input = name, input.JSON()
	return bound, nil
}

func (r *Connection) DecodeRunNow(reply *protocol.CallPluginResponse) (conversation.RunReference, error) {
	if reply == nil {
		return conversation.RunReference{}, runtimeContractViolation("run schedule now returned nil")
	}
	var result SchedulesRunNowV1Output
	if err := json.Unmarshal(reply.Output, &result, json.RejectUnknownMembers(true)); err != nil {
		return conversation.RunReference{}, runtimeContractViolation("run schedule now output: %v", err)
	}
	if err := errors.Join(protocol.ValidateSessionID(result.SessionID), protocol.ValidateRunID(result.RunID)); err != nil {
		return conversation.RunReference{}, runtimeContractViolation("run schedule now identity: %v", err)
	}
	return conversation.RunReference{SessionID: result.SessionID, RunID: result.RunID}, nil
}

func scheduleResult(operation, expectedID string, result *protocol.Schedule, err error) (protocol.Schedule, error) {
	if err != nil {
		return protocol.Schedule{}, classifyError(err)
	}
	if result == nil {
		return protocol.Schedule{}, runtimeContractViolation("%s returned nil", operation)
	}
	if err := requireIdentity(operation, result.ID, expectedID); err != nil {
		return protocol.Schedule{}, err
	}
	return *result, nil
}

// The accepted provider and complete callable contract are captured once for all
// pages. A later selection cannot splice another installation into this read.
func (r *Connection) selectedSchedules(ctx context.Context) (protocol.CallPluginRequest, error) {
	if r.plugins == nil {
		return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
	}
	required, err := capability.Parse([]byte(schedulesCapabilityJSON))
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	providers, err := r.ListPluginProviders(ctx)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	var selected protocol.PluginProviderSelection
	found := false
	for _, provider := range providers {
		if provider.Role == required.Descriptor().Reference {
			selected = provider
			found = true
			break
		}
	}
	if !found {
		return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
	}
	installations, err := r.ListPlugins(ctx)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	for _, installation := range installations {
		if installation.ID != selected.InstallationID {
			continue
		}
		if !installation.Enabled {
			return protocol.CallPluginRequest{}, protocol.ErrPluginDisabled
		}
		for _, contribution := range installation.Selected.Contributions {
			if contribution.ID != selected.Contribution {
				continue
			}
			encoded, err := json.Marshal(contribution.Contract)
			if err != nil {
				return protocol.CallPluginRequest{}, runtimeContractViolation("schedule contribution contract: %v", err)
			}
			actual, err := capability.Parse(encoded)
			if err != nil {
				return protocol.CallPluginRequest{}, errors.Join(protocol.ErrPluginUnavailable, err)
			}
			if actual.Identity() != required.Identity() {
				return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
			}
			return protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: contribution.ID, Contract: schedulesCapabilityJSON, Method: schedulesListV1Method, Target: capability.Target{Scope: required.Descriptor().Scope, Reference: installation.ID}}, nil
		}
	}
	return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
}
