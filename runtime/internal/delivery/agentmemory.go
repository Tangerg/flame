package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/protocol"
)

// Agent Memory operations provide HITL review of the agent's self-maintained
// memory: proposals wait as pending until the user approves or rewrites them;
// only active memory reaches the prompt or the search_memory tool.

// agentMemoryUseCases is Delivery's consumer-side port. Its methods express
// complete review use cases, rather than exposing the domain store workflow.
type agentMemoryUseCases interface {
	Add(ctx context.Context, scope agentmemory.Scope, cwd, content string) (agentmemory.Item, error)
}

// AddAgentMemory stores a user-authored active item (agentMemory.add).
func (s *Handler) AddAgentMemory(ctx context.Context, in protocol.AgentMemoryAddRequest) (*protocol.AgentMemoryItem, error) {
	scope, cwd, err := agentMemoryTargetFromWire(in.Scope, in.Workspace)
	if err != nil {
		return nil, err
	}
	item, err := s.agentMemory.Add(ctx, scope, cwd, in.Content)
	if err != nil {
		return nil, mapAgentMemoryErr(err)
	}
	w, err := presentAgentMemoryItem(item)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// agentMemoryTargetFromWire maps the closed wire target to the Application's
// scope + workspace input. It also protects direct in-process callers: project
// always has a workspace and user never carries one, matching the generated wire
// constraint instead of silently ignoring a half-valid request.
func agentMemoryTargetFromWire(scope protocol.AgentMemoryScope, workspace *protocol.WorkspaceRef) (agentmemory.Scope, string, error) {
	switch scope {
	case protocol.AgentMemoryScopeProject:
		if workspace == nil {
			return "", "", NewFailure(protocol.ErrInvalidParams, "project agent memory requires workspace")
		}
		return agentmemory.ScopeProject, workspace.Path, nil
	case protocol.AgentMemoryScopeUser:
		if workspace != nil {
			return "", "", NewFailure(protocol.ErrInvalidParams, "user agent memory forbids workspace")
		}
		return agentmemory.ScopeUser, "", nil
	default:
		return "", "", NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown agent memory scope %q", scope))
	}
}

func mapAgentMemoryErr(err error) error {
	if errors.Is(err, plugin.ErrCommandUnconfirmed) {
		return err
	}
	if errors.Is(err, plugin.ErrUnavailable) {
		return errors.Join(protocol.ErrPluginUnavailable, err)
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, agentmemory.ErrNotFound), errors.Is(err, agentmemory.ErrNotVisible):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), "no such memory item")
	case errors.Is(err, agentmemory.ErrTargetFull):
		return NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	default:
		return wireWorkspaceError(err)
	}
}

func presentAgentMemoryItem(item agentmemory.Item) (protocol.AgentMemoryItem, error) {
	scope, err := presentAgentMemoryScope(item.Scope())
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	origin, err := presentAgentMemoryOrigin(item.Origin())
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	status, err := presentAgentMemoryStatus(item.Status())
	if err != nil {
		return protocol.AgentMemoryItem{}, err
	}
	return protocol.AgentMemoryItem{
		ID:        item.ID().String(),
		Scope:     scope,
		Content:   item.Content(),
		Origin:    origin,
		Status:    status,
		Pinned:    item.Pinned(),
		SessionID: item.SessionID(),
		Day:       item.Day(),
		CreatedAt: item.CreatedAt(),
		UpdatedAt: item.UpdatedAt(),
	}, nil
}

func presentAgentMemoryScope(scope agentmemory.Scope) (protocol.AgentMemoryScope, error) {
	switch scope {
	case agentmemory.ScopeProject:
		return protocol.AgentMemoryScopeProject, nil
	case agentmemory.ScopeUser:
		return protocol.AgentMemoryScopeUser, nil
	default:
		return "", fmt.Errorf("agentMemory: unsupported scope %q", scope)
	}
}

func presentAgentMemoryOrigin(origin agentmemory.Origin) (protocol.AgentMemoryOrigin, error) {
	switch origin {
	case agentmemory.OriginAuto:
		return protocol.AgentMemoryOriginAuto, nil
	case agentmemory.OriginUser:
		return protocol.AgentMemoryOriginUser, nil
	default:
		return "", fmt.Errorf("agentMemory: unsupported origin %q", origin)
	}
}

func presentAgentMemoryStatus(status agentmemory.Status) (protocol.AgentMemoryStatus, error) {
	switch status {
	case agentmemory.StatusActive:
		return protocol.AgentMemoryStatusActive, nil
	case agentmemory.StatusPending:
		return protocol.AgentMemoryStatusPending, nil
	case agentmemory.StatusRejected:
		return "", errors.New("agentMemory: rejected items must not be projected")
	default:
		return "", fmt.Errorf("agentMemory: unsupported status %q", status)
	}
}
