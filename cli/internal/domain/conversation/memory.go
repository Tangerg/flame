package conversation

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

func ParseMemoryScope(value string) (protocol.AgentMemoryScope, error) {
	scope := protocol.AgentMemoryScope(strings.TrimSpace(value))
	workspace := ""
	if scope == protocol.AgentMemoryScopeProject {
		workspace = "/"
	}
	if _, err := NewMemoryTarget(scope, workspace); err != nil {
		return "", err
	}
	return scope, nil
}

// MemoryTarget couples a scope to exactly the workspace context it requires.
type MemoryTarget struct {
	Scope     protocol.AgentMemoryScope
	Workspace string
}

func NewMemoryTarget(scope protocol.AgentMemoryScope, workspace string) (MemoryTarget, error) {
	target := MemoryTarget{Scope: scope, Workspace: strings.TrimSpace(workspace)}
	return target, target.Validate()
}

func (t MemoryTarget) Validate() error {
	switch t.Scope {
	case protocol.AgentMemoryScopeUser:
		if t.Workspace != "" {
			return fmt.Errorf("agent memory user target cannot carry a workspace")
		}
	case protocol.AgentMemoryScopeProject:
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

// ValidateMemoryItem checks the temporal relationship that Runtime's field-level
// wire contract cannot express. Every field-level rule is the endpoint's answer,
// already given before this item reached the CLI.
func ValidateMemoryItem(item protocol.AgentMemoryItem) error {
	if item.UpdatedAt.Before(item.CreatedAt) {
		return fmt.Errorf("agent memory item %s was updated before creation", item.ID)
	}
	return nil
}
