package runtimebinding

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"

	"github.com/Tangerg/flame/apps/cli/internal/domain/schedule"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
)

const (
	schedulePageLimit = 100
	// maximumSchedulePageRequests bounds full-catalog materialization to at
	// most 10,000 schedules when the Runtime serves the requested page width.
	// The request bound remains authoritative for a Runtime that under-fills.
	maximumSchedulePageRequests = 100
)

func (r *Connection) Schedules(ctx context.Context) ([]schedule.CreateV1Output, error) {
	bound, err := r.selectedInstalledCall(ctx, schedule.CapabilityJSON, schedule.ListV1Method)
	if err != nil {
		return nil, err
	}
	var schedules []schedule.CreateV1Output
	seenIDs := make(map[string]struct{})
	cursors, err := newCursorTraversal("list schedules", "", maximumSchedulePageRequests)
	if err != nil {
		return nil, err
	}
	for {
		cursor := cursors.Current()
		input, err := jsoninput.Encode(schedule.ListV1Input{Cursor: cursor, Limit: new(int64(schedulePageLimit))})
		if err != nil {
			return nil, err
		}
		request := bound
		request.Input = string(input.JSON())
		reply, err := r.CallPlugin(ctx, request, "")
		if err != nil {
			return nil, classifyError(err)
		}
		var page schedule.ListV1Output
		if err := json.Unmarshal([]byte(reply.Output), &page, json.RejectUnknownMembers(true)); err != nil {
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
			schedules = append(schedules, schedule.CreateV1Output(row))
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

func (r *Connection) PrepareCreate(ctx context.Context, request schedule.CreateV1Input) (protocol.CallPluginRequest, error) {
	if request.CWD != "" {
		resolved, err := r.Resolve(ctx, workspace.ResolveRequest{Path: request.CWD})
		if err != nil {
			return protocol.CallPluginRequest{}, fmt.Errorf("create schedule workspace: %w", err)
		}
		request.CWD = resolved.Path
	}
	input, err := jsoninput.Encode(request)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return r.prepareSchedulesCommand(ctx, schedule.CreateV1Method, input)
}

func (r *Connection) DecodeCreate(reply *protocol.CallPluginResponse) (schedule.CreateV1Output, error) {
	if reply == nil {
		return schedule.CreateV1Output{}, runtimeContractViolation("create schedule returned nil")
	}
	var result schedule.CreateV1Output
	if err := json.Unmarshal([]byte(reply.Output), &result, json.RejectUnknownMembers(true)); err != nil {
		return schedule.CreateV1Output{}, runtimeContractViolation("create schedule output: %v", err)
	}
	if result.ID == "" || result.Revision == 0 || result.CreatedAt.IsZero() || strings.TrimSpace(result.Instructions) == "" || result.Cron == "" {
		return schedule.CreateV1Output{}, runtimeContractViolation("schedule snapshot is incomplete")
	}
	return result, nil
}

func (r *Connection) PrepareUpdate(ctx context.Context, request schedule.UpdateV1Input) (protocol.CallPluginRequest, error) {
	if request.CWD != nil && *request.CWD != "" {
		resolved, err := r.Resolve(ctx, workspace.ResolveRequest{Path: *request.CWD})
		if err != nil {
			return protocol.CallPluginRequest{}, fmt.Errorf("update schedule workspace: %w", err)
		}
		request.CWD = new(resolved.Path)
	}
	input, err := jsoninput.Encode(request)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return r.prepareSchedulesCommand(ctx, schedule.UpdateV1Method, input)
}

func (r *Connection) DecodeUpdate(expectedID string, reply *protocol.CallPluginResponse) (schedule.UpdateV1Output, error) {
	value, err := r.DecodeCreate(reply)
	if err != nil {
		return schedule.UpdateV1Output{}, err
	}
	if err := requireIdentity("update schedule", value.ID, expectedID); err != nil {
		return schedule.UpdateV1Output{}, err
	}
	return schedule.UpdateV1Output(value), nil
}

func (r *Connection) PrepareDelete(ctx context.Context, id string) (protocol.CallPluginRequest, error) {
	input, err := jsoninput.Encode(schedule.DeleteV1Input{ID: strings.TrimSpace(id)})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return r.prepareSchedulesCommand(ctx, schedule.DeleteV1Method, input)
}

func (r *Connection) PrepareRunNow(ctx context.Context, id string) (protocol.CallPluginRequest, error) {
	input, err := jsoninput.Encode(schedule.RunNowV1Input{ID: strings.TrimSpace(id)})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return r.prepareSchedulesCommand(ctx, schedule.RunNowV1Method, input)
}

func (r *Connection) prepareSchedulesCommand(ctx context.Context, name string, input jsoninput.Input) (protocol.CallPluginRequest, error) {
	bound, err := r.selectedInstalledCall(ctx, schedule.CapabilityJSON, schedule.ListV1Method)
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
	bound.Method, bound.Input = name, string(input.JSON())
	return bound, nil
}

func (r *Connection) DecodeRunNow(reply *protocol.CallPluginResponse) (conversation.RunReference, error) {
	if reply == nil {
		return conversation.RunReference{}, runtimeContractViolation("run schedule now returned nil")
	}
	var result schedule.RunNowV1Output
	if err := json.Unmarshal([]byte(reply.Output), &result, json.RejectUnknownMembers(true)); err != nil {
		return conversation.RunReference{}, runtimeContractViolation("run schedule now output: %v", err)
	}
	if err := errors.Join(protocol.ValidateSessionID(result.SessionID), protocol.ValidateRunID(result.RunID)); err != nil {
		return conversation.RunReference{}, runtimeContractViolation("run schedule now identity: %v", err)
	}
	return conversation.RunReference{SessionID: result.SessionID, RunID: result.RunID}, nil
}
