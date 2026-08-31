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

type agentMemoryAdapter struct{ runtime *Connection }

var _ agent.MemoryService = (*agentMemoryAdapter)(nil)

func (a *agentMemoryAdapter) Items(ctx context.Context, target agent.MemoryTarget) ([]agent.MemoryItem, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	request := protocol.AgentMemoryListRequest{Scope: protocol.AgentMemoryScope(validated.Scope)}
	if validated.Scope == agent.MemoryProject {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	result, err := r.agentMemory.ListAgentMemory(ctx, request, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, runtimeContractViolation("list agent memory returned nil")
	}
	items := make([]agent.MemoryItem, 0, len(result.Items))
	seen := make(map[string]struct{}, len(result.Items))
	for index, value := range result.Items {
		item := projectAgentMemoryItem(value)
		if err := item.Validate(); err != nil {
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

func (a *agentMemoryAdapter) Review(ctx context.Context, id string, decision agent.MemoryReviewDecision) error {
	r := a.runtime
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("review agent memory: id is empty")
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.agentMemory.ReviewAgentMemory(ctx, protocol.AgentMemoryReviewRequest{
		ID: id, Decision: protocol.AgentMemoryReviewDecision(decision),
	}, options))
}

func (a *agentMemoryAdapter) Update(ctx context.Context, patch agent.MemoryPatch) (agent.MemoryItem, error) {
	r := a.runtime
	if err := patch.Validate(); err != nil {
		return agent.MemoryItem{}, err
	}
	validated := patch
	if patch.Content != nil {
		content := strings.TrimSpace(*patch.Content)
		validated.Content = &content
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.MemoryItem{}, err
	}
	result, err := r.agentMemory.UpdateAgentMemory(ctx, protocol.AgentMemoryUpdateRequest{
		ID: validated.ID, Content: clonePointer(validated.Content), Pinned: clonePointer(validated.Pinned),
	}, options)
	item, err := projectAgentMemoryResult("update agent memory", validated.ID, "", result, err)
	if err != nil {
		return agent.MemoryItem{}, err
	}
	if err := validated.ValidateResult(item); err != nil {
		return agent.MemoryItem{}, runtimeContractViolation("update agent memory returned an invalid acknowledgement: %v", err)
	}
	return item, nil
}

func (a *agentMemoryAdapter) Delete(ctx context.Context, id string) error {
	r := a.runtime
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("delete agent memory: id is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.agentMemory.DeleteAgentMemory(ctx, protocol.AgentMemoryItemRequest{ID: id}, options))
}

func (a *agentMemoryAdapter) Add(ctx context.Context, target agent.MemoryTarget, content string) (agent.MemoryItem, error) {
	r := a.runtime
	validated, err := a.resolveTarget(ctx, target)
	if err != nil {
		return agent.MemoryItem{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return agent.MemoryItem{}, errors.New("add agent memory: content is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.MemoryItem{}, err
	}
	request := protocol.AgentMemoryAddRequest{Scope: protocol.AgentMemoryScope(validated.Scope), Content: content}
	if validated.Scope == agent.MemoryProject {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	result, err := r.agentMemory.AddAgentMemory(ctx, request, options)
	item, err := projectAgentMemoryResult("add agent memory", "", validated.Scope, result, err)
	if err != nil {
		return agent.MemoryItem{}, err
	}
	if err := validated.ValidateAddResult(content, item); err != nil {
		return agent.MemoryItem{}, runtimeContractViolation("add agent memory returned an invalid acknowledgement: %v", err)
	}
	return item, nil
}

func (a *agentMemoryAdapter) resolveTarget(ctx context.Context, target agent.MemoryTarget) (agent.MemoryTarget, error) {
	if err := target.Validate(); err != nil {
		return agent.MemoryTarget{}, err
	}
	if target.Scope != agent.MemoryProject {
		return target, nil
	}
	resolved, err := a.runtime.Resolve(ctx, workspace.ResolveRequest{Path: target.Workspace})
	if err != nil {
		return agent.MemoryTarget{}, fmt.Errorf("resolve agent memory workspace: %w", err)
	}
	return agent.NewMemoryTarget(target.Scope, resolved.Path)
}

func projectAgentMemoryResult(
	operation, expectedID string,
	expectedScope agent.MemoryScope,
	result *protocol.AgentMemoryItem,
	err error,
) (agent.MemoryItem, error) {
	if err != nil {
		return agent.MemoryItem{}, classifyError(err)
	}
	if result == nil {
		return agent.MemoryItem{}, runtimeContractViolation("%s returned nil", operation)
	}
	item := projectAgentMemoryItem(*result)
	if err := item.Validate(); err != nil {
		return agent.MemoryItem{}, runtimeContractViolation("%s returned an invalid item: %v", operation, err)
	}
	if expectedID != "" && item.ID != expectedID {
		return agent.MemoryItem{}, runtimeContractViolation("%s returned id %q for %q", operation, item.ID, expectedID)
	}
	if expectedScope != "" && item.Scope != expectedScope {
		return agent.MemoryItem{}, runtimeContractViolation("%s returned %s scope, want %s", operation, item.Scope, expectedScope)
	}
	return item, nil
}

func projectAgentMemoryItem(value protocol.AgentMemoryItem) agent.MemoryItem {
	return agent.MemoryItem{
		ID: value.ID, Scope: agent.MemoryScope(value.Scope), Content: value.Content,
		Origin: agent.MemoryOrigin(value.Origin), Status: agent.MemoryStatus(value.Status), Pinned: value.Pinned,
		SessionID: value.SessionID, Day: value.Day, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}
