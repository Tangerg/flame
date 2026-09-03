package runtimebinding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type agentMemoryBinding interface {
	ListAgentMemory(context.Context, protocol.AgentMemoryListRequest, flameruntime.CallOptions) (*protocol.AgentMemoryList, error)
	ReviewAgentMemory(context.Context, protocol.AgentMemoryReviewRequest, flameruntime.CommandOptions) error
	UpdateAgentMemory(context.Context, protocol.AgentMemoryUpdateRequest, flameruntime.CommandOptions) (*protocol.AgentMemoryItem, error)
	DeleteAgentMemory(context.Context, protocol.AgentMemoryItemRequest, flameruntime.CommandOptions) error
	AddAgentMemory(context.Context, protocol.AgentMemoryAddRequest, flameruntime.CommandOptions) (*protocol.AgentMemoryItem, error)
}

type AgentMemory struct{ runtime *Connection }

func (a *AgentMemory) Items(ctx context.Context, target agent.MemoryTarget) ([]protocol.AgentMemoryItem, error) {
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
	result, err := r.agentMemory.ListAgentMemory(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, runtimeContractViolation("list agent memory returned nil")
	}
	items := make([]protocol.AgentMemoryItem, 0, len(result.Items))
	seen := make(map[string]struct{}, len(result.Items))
	for index, value := range result.Items {
		item := value
		if err := agent.ValidateMemoryItem(item); err != nil {
			return nil, runtimeContractViolation("list agent memory item %d is invalid: %v", index+1, err)
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
	return items, nil
}

func (a *AgentMemory) Review(ctx context.Context, id string, decision protocol.AgentMemoryReviewDecision) error {
	r := a.runtime
	id = strings.TrimSpace(id)
	request := protocol.AgentMemoryReviewRequest{ID: id, Decision: decision}
	if err := request.ValidateWire(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.agentMemory.ReviewAgentMemory(ctx, request, options))
}

func (a *AgentMemory) Update(ctx context.Context, patch agent.MemoryPatch) (protocol.AgentMemoryItem, error) {
	r := a.runtime
	if err := patch.Validate(); err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	validated := patch
	if patch.Content != nil {
		content := strings.TrimSpace(*patch.Content)
		validated.Content = &content
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	result, err := r.agentMemory.UpdateAgentMemory(ctx, protocol.AgentMemoryUpdateRequest{
		ID: validated.ID, Content: clonePointer(validated.Content), Pinned: clonePointer(validated.Pinned),
	}, options)
	item, err := agentMemoryResult("update agent memory", validated.ID, "", result, err)
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	if err := validated.ValidateResult(item); err != nil {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("update agent memory returned an invalid acknowledgement: %v", err)
	}
	return item, nil
}

func (a *AgentMemory) Delete(ctx context.Context, id string) error {
	r := a.runtime
	id = strings.TrimSpace(id)
	request := protocol.AgentMemoryItemRequest{ID: id}
	if err := request.ValidateWire(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.agentMemory.DeleteAgentMemory(ctx, request, options))
}

func (a *AgentMemory) Add(ctx context.Context, target agent.MemoryTarget, content string) (protocol.AgentMemoryItem, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return protocol.AgentMemoryItem{}, errors.New("add agent memory: content is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	request := protocol.AgentMemoryAddRequest{Scope: validated.Scope, Content: content}
	if validated.Scope == protocol.AgentMemoryScopeProject {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	if err := protocol.ValidateWireTree(request); err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	result, err := r.agentMemory.AddAgentMemory(ctx, request, options)
	item, err := agentMemoryResult("add agent memory", "", validated.Scope, result, err)
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	if err := validated.ValidateAddResult(content, item); err != nil {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("add agent memory returned an invalid acknowledgement: %v", err)
	}
	return item, nil
}

func (a *AgentMemory) resolveTarget(ctx context.Context, target agent.MemoryTarget) (agent.MemoryTarget, error) {
	if err := target.Validate(); err != nil {
		return agent.MemoryTarget{}, err
	}
	if target.Scope != protocol.AgentMemoryScopeProject {
		return target, nil
	}
	resolved, err := a.runtime.Resolve(ctx, workspace.ResolveRequest{Path: target.Workspace})
	if err != nil {
		return agent.MemoryTarget{}, fmt.Errorf("resolve agent memory workspace: %w", err)
	}
	return agent.NewMemoryTarget(target.Scope, resolved.Path)
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
	if err := agent.ValidateMemoryItem(item); err != nil {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("%s returned an invalid item: %v", operation, err)
	}
	if expectedID != "" && item.ID != expectedID {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("%s returned id %q for %q", operation, item.ID, expectedID)
	}
	if expectedScope != "" && item.Scope != expectedScope {
		return protocol.AgentMemoryItem{}, runtimeContractViolation("%s returned %s scope, want %s", operation, item.Scope, expectedScope)
	}
	return item, nil
}
