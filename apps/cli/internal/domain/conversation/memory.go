package conversation

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func ParseMemoryScope(value string) (memory.UpdateV2OutputScope, error) {
	scope := memory.UpdateV2OutputScope(strings.TrimSpace(value))
	workspace := ""
	if scope == memory.UpdateV2OutputScopeProject {
		workspace = "/"
	}
	if _, err := NewMemoryTarget(scope, workspace); err != nil {
		return "", err
	}
	return scope, nil
}

// MemoryTarget couples a scope to exactly the workspace context it requires.
type MemoryTarget struct {
	Scope     memory.UpdateV2OutputScope
	Workspace string
}

func NewMemoryTarget(scope memory.UpdateV2OutputScope, workspace string) (MemoryTarget, error) {
	target := MemoryTarget{Scope: scope, Workspace: workspace}
	return target, target.Validate()
}

func (t MemoryTarget) Validate() error {
	switch t.Scope {
	case memory.UpdateV2OutputScopeUser:
		if t.Workspace != "" {
			return fmt.Errorf("agent memory user target cannot carry a workspace")
		}
	case memory.UpdateV2OutputScopeProject:
		if t.Workspace == "" {
			return fmt.Errorf("agent memory project target requires a workspace")
		}
		if err := protocol.ValidateWireTree((protocol.WorkspaceRef{Path: t.Workspace})); err != nil {
			return fmt.Errorf("agent memory workspace: %w", err)
		}
	default:
		return fmt.Errorf("unknown agent memory scope %q", t.Scope)
	}
	return nil
}
