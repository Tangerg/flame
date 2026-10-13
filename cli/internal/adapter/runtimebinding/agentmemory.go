package runtimebinding

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/memory"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

type AgentMemory struct{ runtime *Connection }

func (a *AgentMemory) Items(ctx context.Context, target conversation.MemoryTarget) ([]memory.Item, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	bound, err := r.selectedInstalledCall(ctx, memory.CapabilityJSON, memory.ListV1Method)
	if err != nil {
		return nil, err
	}
	input := memory.ListV1Input{Scope: string(validated.Scope), CWD: validated.Workspace}
	var traversal *cursorTraversal
	capacity := int64(0)
	items := make([]memory.Item, 0)
	seen := make(map[string]struct{})
	for {
		if traversal != nil {
			input.Cursor = traversal.Current()
		}
		encoded, err := jsoninput.Encode(input)
		if err != nil {
			return nil, err
		}
		request := bound
		request.Input = encoded.JSON()
		reply, err := r.CallPlugin(ctx, request, "")
		if err != nil {
			return nil, classifyError(err)
		}
		var result memory.ListV1Output
		if err := json.Unmarshal(reply.Output, &result, json.RejectUnknownMembers(true)); err != nil {
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
			item := memoryProjection(row)
			if err := conversation.ValidateMemoryItem(item); err != nil {
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
		more, err := traversal.Advance(result.NextCursor)
		if err != nil {
			return nil, err
		}
		if !more {
			break
		}
	}
	return items, nil
}

func (a *AgentMemory) PrepareReview(ctx context.Context, id string, decision memory.ReviewDecision) (protocol.CallPluginRequest, error) {
	input, err := jsoninput.Encode(memory.ReviewV1Input{ID: strings.TrimSpace(id), Decision: string(decision)})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return a.prepareCommand(ctx, memory.ReviewV1Method, input)
}

func (a *AgentMemory) PrepareUpdate(ctx context.Context, request memory.UpdateV1Input) (protocol.CallPluginRequest, error) {
	input, err := jsoninput.Encode(request)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return a.prepareCommand(ctx, memory.UpdateV1Method, input)
}

func (a *AgentMemory) DecodeUpdate(id string, reply *protocol.CallPluginResponse) (memory.Item, error) {
	if reply == nil {
		return memory.Item{}, runtimeContractViolation("update agent memory returned nil")
	}
	var output memory.UpdateV1Output
	if err := json.Unmarshal(reply.Output, &output, json.RejectUnknownMembers(true)); err != nil {
		return memory.Item{}, runtimeContractViolation("update agent memory output: %v", err)
	}
	item := memoryProjection(output)
	return agentMemoryResult("update agent memory", id, "", &item, nil)
}

func memoryProjection(row memory.UpdateV1Output) memory.Item {
	return memory.Item{ID: row.ID, Scope: memory.Scope(row.Scope), Content: row.Content, Origin: memory.Origin(row.Origin), Status: memory.Status(row.Status), Pinned: row.Pinned, SessionID: row.SessionID, Day: row.Day, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}

func (a *AgentMemory) PrepareDelete(ctx context.Context, id string) (protocol.CallPluginRequest, error) {
	input, err := jsoninput.Encode(memory.DeleteV1Input{ID: strings.TrimSpace(id)})
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return a.prepareCommand(ctx, memory.DeleteV1Method, input)
}

func (a *AgentMemory) prepareCommand(ctx context.Context, name string, input jsoninput.Input) (protocol.CallPluginRequest, error) {
	bound, err := a.runtime.selectedInstalledCall(ctx, memory.CapabilityJSON, name)
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
		return protocol.CallPluginRequest{}, fmt.Errorf("agent memory command: %w", err)
	}
	bound.Input = input.JSON()
	return bound, nil
}

func (a *AgentMemory) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, commandID replay.CommandID) (*protocol.CallPluginResponse, error) {
	return a.runtime.CallPlugin(ctx, request, commandID)
}

func (a *AgentMemory) PrepareAdd(ctx context.Context, request memory.AddV1Input) (protocol.CallPluginRequest, error) {
	if _, err := conversation.NewMemoryTarget(memory.Scope(request.Scope), request.CWD); err != nil {
		return protocol.CallPluginRequest{}, err
	}
	input, err := jsoninput.Encode(request)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return a.prepareCommand(ctx, memory.AddV1Method, input)
}

func (a *AgentMemory) DecodeAdd(reply *protocol.CallPluginResponse) (memory.AddV1Output, error) {
	if reply == nil {
		return memory.AddV1Output{}, runtimeContractViolation("add agent memory returned nil")
	}
	var output memory.AddV1Output
	if err := json.Unmarshal(reply.Output, &output, json.RejectUnknownMembers(true)); err != nil {
		return memory.AddV1Output{}, runtimeContractViolation("add agent memory output: %v", err)
	}
	item := memoryProjection(output.Item)
	if _, err := agentMemoryResult("add agent memory", "", "", &item, nil); err != nil {
		return memory.AddV1Output{}, err
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
	if err := conversation.ValidateMemoryItem(item); err != nil {
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
