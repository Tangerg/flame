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
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

type agentMemoryBinding interface {
	AddAgentMemory(context.Context, protocol.AgentMemoryAddRequest, flameruntime.CommandOptions) (*protocol.AgentMemoryItem, error)
}

type AgentMemory struct{ runtime *Connection }

func (a *AgentMemory) Items(ctx context.Context, target conversation.MemoryTarget) ([]protocol.AgentMemoryItem, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	bound, err := r.selectedInstalledCall(ctx, memory.CapabilityJSON, memory.ListV1Method)
	if err != nil {
		return nil, err
	}
	input := memory.ListV1Input{Scope: string(validated.Scope), CWD: validated.Workspace, Limit: new(int64(100))}
	traversal, err := newCursorTraversal("list agent memory", "", flameruntime.MaximumAgentMemoryItemsPerTarget)
	if err != nil {
		return nil, err
	}
	items := make([]protocol.AgentMemoryItem, 0)
	seen := make(map[string]struct{})
	for {
		input.Cursor = traversal.Current()
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
		if len(result.Rows) > 100 || len(items)+len(result.Rows) > flameruntime.MaximumAgentMemoryItemsPerTarget || (len(result.Rows) == 0 && result.NextCursor != "") {
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

func (a *AgentMemory) DecodeUpdate(id string, reply *protocol.CallPluginResponse) (protocol.AgentMemoryItem, error) {
	if reply == nil {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("update agent memory returned nil")
	}
	var output memory.UpdateV1Output
	if err := json.Unmarshal(reply.Output, &output, json.RejectUnknownMembers(true)); err != nil {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("update agent memory output: %v", err)
	}
	item := memoryProjection(output)
	if err := protocol.ValidateWireTree(item); err != nil {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("update agent memory projection: %v", err)
	}
	return agentMemoryResult("update agent memory", id, "", &item, nil)
}

func memoryProjection(row memory.UpdateV1Output) protocol.AgentMemoryItem {
	return protocol.AgentMemoryItem{ID: row.ID, Scope: protocol.AgentMemoryScope(row.Scope), Content: row.Content, Origin: protocol.AgentMemoryOrigin(row.Origin), Status: protocol.AgentMemoryStatus(row.Status), Pinned: row.Pinned, SessionID: row.SessionID, Day: row.Day, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
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

func (a *AgentMemory) Add(ctx context.Context, target conversation.MemoryTarget, content string) (protocol.AgentMemoryItem, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	options := r.commandOptions()
	request := protocol.AgentMemoryAddRequest{Scope: validated.Scope, Content: content}
	if validated.Scope == protocol.AgentMemoryScopeProject {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	result, err := r.agentMemory.AddAgentMemory(ctx, request, options)
	return agentMemoryResult("add agent memory", "", validated.Scope, result, err)
}

func (a *AgentMemory) resolveTarget(ctx context.Context, target conversation.MemoryTarget) (conversation.MemoryTarget, error) {
	if err := target.Validate(); err != nil {
		return conversation.MemoryTarget{}, err
	}
	if target.Scope != protocol.AgentMemoryScopeProject {
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
	expectedScope protocol.AgentMemoryScope,
	result *protocol.AgentMemoryItem,
	err error,
) (protocol.AgentMemoryItem, error) {
	if err != nil {
		return protocol.AgentMemoryItem{}, classifyError(err)
	}
	if result == nil {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("%s returned nil", operation)
	}
	item := *result
	if err := conversation.ValidateMemoryItem(item); err != nil {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("%s returned an invalid item: %v", operation, err)
	}
	if err := requireIdentity(operation, item.ID, expectedID); err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	if expectedScope != "" && item.Scope != expectedScope {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("%s returned %s scope, want %s", operation, item.Scope, expectedScope)
	}
	return item, nil
}
