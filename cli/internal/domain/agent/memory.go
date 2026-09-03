package agent

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

func ParseMemoryScope(value string) (protocol.AgentMemoryScope, error) {
	scope := protocol.AgentMemoryScope(strings.TrimSpace(value))
	workspace := ""
	if scope == protocol.AgentMemoryScopeProject {
		workspace = string(filepath.Separator)
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
	if t.Scope == protocol.AgentMemoryScopeProject && !filepath.IsAbs(t.Workspace) {
		return errors.New("project agent memory workspace is not absolute")
	}
	return nil
}

// ValidateMemoryItem checks the temporal relationship that Runtime's field-level
// wire contract cannot express.
func ValidateMemoryItem(item protocol.AgentMemoryItem) error {
	if err := item.ValidateWire(); err != nil {
		return err
	}
	if item.UpdatedAt.Before(item.CreatedAt) {
		return fmt.Errorf("agent memory item %s was updated before creation", item.ID)
	}
	return nil
}

func (t MemoryTarget) ValidateAddResult(content string, result protocol.AgentMemoryItem) error {
	if err := t.Validate(); err != nil {
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("add agent memory: content is empty")
	}
	var problems []error
	if err := ValidateMemoryItem(result); err != nil {
		problems = append(problems, fmt.Errorf("runtime result: %w", err))
	}
	if result.Scope != t.Scope {
		problems = append(problems, fmt.Errorf("runtime returned %s scope, want %s", result.Scope, t.Scope))
	}
	if result.Content != content {
		problems = append(problems, fmt.Errorf("runtime returned content %q, want %q", result.Content, content))
	}
	if result.Origin != protocol.AgentMemoryOriginUser || result.Status != protocol.AgentMemoryStatusActive {
		problems = append(problems, fmt.Errorf(
			"runtime returned %s/%s provenance, want %s/%s",
			result.Origin, result.Status, protocol.AgentMemoryOriginUser, protocol.AgentMemoryStatusActive,
		))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("add agent memory: %w", err)
	}
	return nil
}
