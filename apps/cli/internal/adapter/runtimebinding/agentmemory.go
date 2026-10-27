package runtimebinding

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
)

type AgentMemory struct{ runtime *Connection }

func (a *AgentMemory) Items(ctx context.Context, target conversation.MemoryTarget) ([]memory.Item, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	bound, err := r.selectedInstalledCall(ctx, memory.CapabilityJSON, memory.ListV2Method)
	if err != nil {
		return nil, err
	}
	input := memory.ListV2Input{Scope: string(validated.Scope), CWD: validated.Workspace}
	var traversal *cursorTraversal
	capacity := int64(0)
	items := make([]memory.Item, 0)
	seen := make(map[string]struct{})
	for {
		if traversal != nil {
			input.Cursor = traversal.cursor()
		}
		encoded, err := jsonvalue.Encode(input)
		if err != nil {
			return nil, err
		}
		request := bound
		request.Input = string(encoded.JSON())
		reply, err := r.CallPlugin(ctx, request, "")
		if err != nil {
			return nil, classifyError(err)
		}
		var result memory.ListV2Output
		if err := json.Unmarshal([]byte(reply.Output), &result, json.RejectUnknownMembers(true)); err != nil {
			return nil, runtimeContractViolation("list agent memory output: %v", err)
		}
		if result.Capacity <= 0 || (capacity != 0 && result.Capacity != capacity) {
			return nil, runtimeContractViolation("list agent memory returned an invalid or changed capacity")
		}
		if traversal == nil {
			capacity = result.Capacity
			traversal, err = newCursorTraversal("list agent memory", "", int(capacity))
			if err != nil {
				return nil, err
			}
		}
		if int64(len(items))+int64(len(result.Rows)) > capacity || (len(result.Rows) == 0 && result.NextCursor != "") {
			return nil, runtimeContractViolation("list agent memory exceeded its target capacity or returned an empty continuation")
		}
		for _, row := range result.Rows {
			item := memoryProjection(memory.UpdateV2Output(row))
			if err := item.Validate(); err != nil {
				return nil, runtimeContractViolation("list agent memory item %d is invalid: %v", len(items)+1, err)
			}
			if item.Scope != validated.Scope {
				return nil, runtimeContractViolation("list agent memory item %s belongs to %s, want %s", item.ID, item.Scope, validated.Scope)
			}
			if _, duplicate := seen[item.ID]; duplicate {
				return nil, runtimeContractViolation("list agent memory repeats %q", item.ID)
			}
			seen[item.ID] = struct{}{}
			items = append(items, item)
		}
		more, err := traversal.advance(result.NextCursor)
		if err != nil {
			return nil, err
		}
		if !more {
			break
		}
	}
	return items, nil
}

func (a *AgentMemory) PrepareReview(ctx context.Context, id string, expectedRevision uint64, decision memory.ReviewDecision) (protocol.CallPluginRequest, error) {
	input, err := jsonvalue.Encode(memory.ReviewV2Input{ID: strings.TrimSpace(id), ExpectedRevision: expectedRevision, Decision: string(decision)})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return a.prepareCommand(ctx, memory.ReviewV2Method, input)
}

func (a *AgentMemory) PrepareUpdate(ctx context.Context, request memory.UpdateV2Input) (protocol.CallPluginRequest, error) {
	input, err := jsonvalue.Encode(request)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return a.prepareCommand(ctx, memory.UpdateV2Method, input)
}

func (a *AgentMemory) DecodeUpdate(id string, reply *protocol.CallPluginResponse) (memory.Item, error) {
	if reply == nil {
		return memory.Item{}, runtimeContractViolation("update agent memory returned nil")
	}
	var output memory.UpdateV2Output
	if err := json.Unmarshal([]byte(reply.Output), &output, json.RejectUnknownMembers(true)); err != nil {
		return memory.Item{}, runtimeContractViolation("update agent memory output: %v", err)
	}
	item := memoryProjection(output)
	return agentMemoryResult("update agent memory", id, "", &item, nil)
}

func memoryProjection(row memory.UpdateV2Output) memory.Item {
	return memory.Item{ID: string(row.ID), Revision: row.Revision, Scope: memory.Scope(row.Scope), Content: row.Content, Origin: memory.Origin(row.Origin), Status: memory.Status(row.Status), Pinned: row.Pinned, SessionID: row.SessionID, Day: row.Day, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func (a *AgentMemory) PrepareDelete(ctx context.Context, id string, expectedRevision uint64) (protocol.CallPluginRequest, error) {
	input, err := jsonvalue.Encode(memory.DeleteV2Input{ID: strings.TrimSpace(id), ExpectedRevision: expectedRevision})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return a.prepareCommand(ctx, memory.DeleteV2Method, input)
}

func (a *AgentMemory) prepareCommand(ctx context.Context, name string, input jsonvalue.Value) (protocol.CallPluginRequest, error) {
	bound, err := a.runtime.selectedInstalledCall(ctx, memory.CapabilityJSON, name)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	contract, err := portable.ParseCapability([]byte(bound.Contract))
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	method, found := contract.Method(name)
	if !found {
		return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
	}
	if err := method.ValidateInput(input); err != nil {
		return protocol.CallPluginRequest{}, fmt.Errorf("agent memory command: %w", err)
	}
	bound.Input = string(input.JSON())
	return bound, nil
}

func (a *AgentMemory) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, commandID authoring.CommandID) (*protocol.CallPluginResponse, error) {
	return a.runtime.CallPlugin(ctx, request, commandID)
}

func (a *AgentMemory) PrepareAdd(ctx context.Context, request memory.AddV2Input) (protocol.CallPluginRequest, error) {
	if _, err := conversation.NewMemoryTarget(memory.Scope(request.Scope), request.CWD); err != nil {
		return protocol.CallPluginRequest{}, err
	}
	input, err := jsonvalue.Encode(request)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return a.prepareCommand(ctx, memory.AddV2Method, input)
}

func (a *AgentMemory) DecodeAdd(reply *protocol.CallPluginResponse) (memory.AddV2Output, error) {
	if reply == nil {
		return memory.AddV2Output{}, runtimeContractViolation("add agent memory returned nil")
	}
	var output memory.AddV2Output
	if err := json.Unmarshal([]byte(reply.Output), &output, json.RejectUnknownMembers(true)); err != nil {
		return memory.AddV2Output{}, runtimeContractViolation("add agent memory output: %v", err)
	}
	item := memoryProjection(memory.UpdateV2Output(output.Item))
	if _, err := agentMemoryResult("add agent memory", "", "", &item, nil); err != nil {
		return memory.AddV2Output{}, err
	}
	return output, nil
}

func (a *AgentMemory) resolveTarget(ctx context.Context, target conversation.MemoryTarget) (conversation.MemoryTarget, error) {
	if err := target.Validate(); err != nil {
		return conversation.MemoryTarget{}, err
	}
	if target.Scope != memory.ScopeProject {
		return target, nil
	}
	resolved, err := a.runtime.Resolve(ctx, workspace.ResolveRequest{Path: target.Workspace})
	if err != nil {
		return conversation.MemoryTarget{}, fmt.Errorf("resolve agent memory workspace: %w", err)
	}
	return conversation.NewMemoryTarget(target.Scope, resolved.Path)
}

func agentMemoryResult(
	operation, expectedID string,
	expectedScope memory.Scope,
	result *memory.Item,
	err error,
) (memory.Item, error) {
	if err != nil {
		return memory.Item{}, classifyError(err)
	}
	if result == nil {
		return memory.Item{}, runtimeContractViolation("%s returned nil", operation)
	}
	item := *result
	if err := item.Validate(); err != nil {
		return memory.Item{}, runtimeContractViolation("%s returned an invalid item: %v", operation, err)
	}
	if err := requireIdentity(operation, item.ID, expectedID); err != nil {
		return memory.Item{}, err
	}
	if expectedScope != "" && item.Scope != expectedScope {
		return memory.Item{}, runtimeContractViolation("%s returned %s scope, want %s", operation, item.Scope, expectedScope)
	}
	return item, nil
}
