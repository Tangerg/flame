package runtimebinding

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type agentMemoryBinding interface {
	ListAgentMemory(context.Context, protocol.AgentMemoryListRequest, flameruntime.CallOptions) (*protocol.Page[protocol.AgentMemoryItem], error)
	ReviewAgentMemory(context.Context, protocol.AgentMemoryReviewRequest, flameruntime.CommandOptions) error
	UpdateAgentMemory(context.Context, protocol.AgentMemoryUpdateRequest, flameruntime.CommandOptions) (*protocol.AgentMemoryItem, error)
	DeleteAgentMemory(context.Context, protocol.AgentMemoryItemRequest, flameruntime.CommandOptions) error
	AddAgentMemory(context.Context, protocol.AgentMemoryAddRequest, flameruntime.CommandOptions) (*protocol.AgentMemoryItem, error)
}

type AgentMemory struct{ runtime *Connection }

func (a *AgentMemory) Items(ctx context.Context, target conversation.MemoryTarget) ([]protocol.AgentMemoryItem, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	request := protocol.AgentMemoryListRequest{Scope: validated.Scope}
	if validated.Scope == protocol.AgentMemoryScopeProject {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return nil, err
	}
	traversal, err := newCursorTraversal("list agent memory", "", flameruntime.MaximumAgentMemoryItemsPerTarget)
	if err != nil {
		return nil, err
	}
	items := make([]protocol.AgentMemoryItem, 0)
	seen := make(map[string]struct{})
	for {
		request.Cursor = traversal.Current()
		result, err := r.agentMemory.ListAgentMemory(ctx, request, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if result == nil {
			return nil, runtimeContractViolation("list agent memory returned nil")
		}
		if len(items)+len(result.Data) > flameruntime.MaximumAgentMemoryItemsPerTarget || (len(result.Data) == 0 && result.NextCursor != "") {
			return nil, runtimeContractViolation("list agent memory exceeded its target capacity or returned an empty continuation")
		}
		for _, item := range result.Data {
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

func (a *AgentMemory) Review(ctx context.Context, id string, decision protocol.AgentMemoryReviewDecision) error {
	r := a.runtime
	id = strings.TrimSpace(id)
	request := protocol.AgentMemoryReviewRequest{ID: id, Decision: decision}
	if err := request.ValidateWire(); err != nil {
		return err
	}
	options := r.commandOptions()
	return classifyError(r.agentMemory.ReviewAgentMemory(ctx, request, options))
}

func (a *AgentMemory) Update(ctx context.Context, request protocol.AgentMemoryUpdateRequest) (protocol.AgentMemoryItem, error) {
	r := a.runtime
	if err := request.ValidateWire(); err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	options := r.commandOptions()
	result, err := r.agentMemory.UpdateAgentMemory(ctx, request, options)
	return agentMemoryResult("update agent memory", request.ID, "", result, err)
}

func (a *AgentMemory) Delete(ctx context.Context, id string) error {
	r := a.runtime
	id = strings.TrimSpace(id)
	request := protocol.AgentMemoryItemRequest{ID: id}
	if err := request.ValidateWire(); err != nil {
		return err
	}
	options := r.commandOptions()
	return classifyError(r.agentMemory.DeleteAgentMemory(ctx, request, options))
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
