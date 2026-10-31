package conversation

import (
	"fmt"
	"strings"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func ParseMemoryScope(value string) (memorycontract.ItemSnapshotScope, error) {
	scope := memorycontract.ItemSnapshotScope(strings.TrimSpace(value))
	workspace := ""
	if scope == memorycontract.ItemSnapshotScopeProject {
		workspace = "/"
	}
	if _, err := NewMemoryTarget(scope, workspace); err != nil {
		return "", err
	}
	return scope, nil
}

// MemoryTarget couples a scope to exactly the workspace context it requires.
type MemoryTarget struct {
	Scope     memorycontract.ItemSnapshotScope
	Workspace string
}

func NewMemoryTarget(scope memorycontract.ItemSnapshotScope, workspace string) (MemoryTarget, error) {
	target := MemoryTarget{Scope: scope, Workspace: workspace}
	return target, target.Validate()
}

func (t MemoryTarget) Validate() error {
	switch t.Scope {
	case memorycontract.ItemSnapshotScopeUser:
		if t.Workspace != "" {
			return fmt.Errorf("agent memory user target cannot carry a workspace")
		}
	case memorycontract.ItemSnapshotScopeProject:
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
