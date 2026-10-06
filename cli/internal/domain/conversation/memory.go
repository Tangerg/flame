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
	var workspace *protocol.WorkspaceRef
	if t.Workspace != "" {
		workspace = &protocol.WorkspaceRef{Path: t.Workspace}
	}
	if err := protocol.ValidateWireTree(protocol.AgentMemoryListRequest{Scope: t.Scope, Workspace: workspace}); err != nil {
		return fmt.Errorf("agent memory target: %w", err)
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
