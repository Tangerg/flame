package conversation

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
	"github.com/Tangerg/flame/runtime/protocol"
)

func ParseMemoryScope(value string) (memory.Scope, error) {
	scope := memory.Scope(strings.TrimSpace(value))
	workspace := ""
	if scope == memory.ScopeProject {
		workspace = "/"
	}
	if _, err := NewMemoryTarget(scope, workspace); err != nil {
		return "", err
	}
	return scope, nil
}

// MemoryTarget couples a scope to exactly the workspace context it requires.
type MemoryTarget struct {
	Scope     memory.Scope
	Workspace string
}

func NewMemoryTarget(scope memory.Scope, workspace string) (MemoryTarget, error) {
	target := MemoryTarget{Scope: scope, Workspace: strings.TrimSpace(workspace)}
	return target, target.Validate()
}

func (t MemoryTarget) Validate() error {
	switch t.Scope {
	case memory.ScopeUser:
		if t.Workspace != "" {
			return fmt.Errorf("agent memory user target cannot carry a workspace")
		}
	case memory.ScopeProject:
		if t.Workspace == "" {
			return fmt.Errorf("agent memory project target requires a workspace")
		}
		if err := (protocol.WorkspaceRef{Path: t.Workspace}).ValidateWire(); err != nil {
			return fmt.Errorf("agent memory workspace: %w", err)
		}
	default:
		return fmt.Errorf("unknown agent memory scope %q", t.Scope)
	}
	return nil
}

func ValidateMemoryItem(item memory.Item) error {
	if item.ID == "" || strings.TrimSpace(item.Content) == "" {
		return fmt.Errorf("agent memory item requires identity and content")
	}
	if item.Scope != memory.ScopeProject && item.Scope != memory.ScopeUser {
		return fmt.Errorf("agent memory item has unknown scope %q", item.Scope)
	}
	if item.Origin != memory.OriginAuto && item.Origin != memory.OriginUser {
		return fmt.Errorf("agent memory item has unknown origin %q", item.Origin)
	}
	if item.Status != memory.StatusActive && item.Status != memory.StatusPending || item.Origin == memory.OriginUser && item.Status != memory.StatusActive {
		return fmt.Errorf("agent memory item has an invalid visible status %q", item.Status)
	}
	if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		return fmt.Errorf("agent memory item requires timestamps")
	}
	if item.UpdatedAt.Before(item.CreatedAt) {
		return fmt.Errorf("agent memory item %s was updated before creation", item.ID)
	}
	return nil
}
