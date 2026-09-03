package workspace

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

type HookCatalog struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []protocol.HookInfo
}

func (c HookCatalog) Validate() error {
	projectHooks := false
	for index, hook := range c.Hooks {
		if err := hook.ValidateWire(); err != nil {
			return fmt.Errorf("hook %d: %w", index+1, err)
		}
		if hook.Matcher != "" {
			if _, err := path.Match(hook.Matcher, ""); err != nil {
				return fmt.Errorf("hook %d matcher %q is invalid: %w", index+1, hook.Matcher, err)
			}
		}
		if hook.Scope == protocol.HookScopeGlobal && !hook.Active {
			return fmt.Errorf("global hook %d is inactive", index+1)
		}
		if hook.Scope == protocol.HookScopeProject {
			projectHooks = true
			if hook.Active != c.ProjectTrusted {
				return fmt.Errorf("project hook %d active state disagrees with trust", index+1)
			}
		}
	}
	if (projectHooks || c.ProjectTrusted) && strings.TrimSpace(c.ProjectRoot) == "" {
		return errors.New("project hooks or trust require a project root")
	}
	return nil
}

// ValidateTrustAcknowledgement proves that an authoritative catalog read after
// SetProjectTrust describes the exact project and trust decision requested.
func (c HookCatalog) ValidateTrustAcknowledgement(projectRoot string, trusted bool) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.ProjectRoot != projectRoot {
		return fmt.Errorf("project hook root is %q, want %q", c.ProjectRoot, projectRoot)
	}
	if c.ProjectTrusted != trusted {
		return fmt.Errorf("project hook trust is %t, want %t", c.ProjectTrusted, trusted)
	}
	return nil
}
