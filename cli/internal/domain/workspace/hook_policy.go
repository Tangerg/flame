package workspace

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/protocol"
)

type HookCatalog struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []protocol.HookInfo
}

// ValidateTrustAcknowledgement proves that an authoritative catalog read after
// SetProjectTrust describes the exact project and trust decision requested.
func (c HookCatalog) ValidateTrustAcknowledgement(projectRoot string, trusted bool) error {
	if c.ProjectRoot != projectRoot {
		return fmt.Errorf("project hook root is %q, want %q", c.ProjectRoot, projectRoot)
	}
	if c.ProjectTrusted != trusted {
		return fmt.Errorf("project hook trust is %t, want %t", c.ProjectTrusted, trusted)
	}
	return nil
}
