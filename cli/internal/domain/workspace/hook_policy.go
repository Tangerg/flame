package workspace

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

type HookCatalog struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []protocol.HookInfo
}

func (c HookCatalog) Validate() error {
	if c.ProjectRoot != "" && !canonicalAbsoluteHookPath(c.ProjectRoot) {
		return fmt.Errorf("project hook root %q is not a canonical absolute path", c.ProjectRoot)
	}
	projectHooks := false
	previousPhase := -1
	for index, hook := range c.Hooks {
		if err := hook.ValidateWire(); err != nil {
			return fmt.Errorf("hook %d: %w", index+1, err)
		}
		if !canonicalAbsoluteHookPath(hook.Source) {
			return fmt.Errorf("hook %d source %q is not a canonical absolute path", index+1, hook.Source)
		}
		if hook.Matcher != "" {
			if _, err := path.Match(hook.Matcher, ""); err != nil {
				return fmt.Errorf("hook %d matcher %q is invalid: %w", index+1, hook.Matcher, err)
			}
		}
		if hook.Scope == protocol.HookScopeGlobal && !hook.Active {
			return fmt.Errorf("global hook %d is inactive", index+1)
		}
		phase := 0
		if hook.Scope == protocol.HookScopeProject {
			phase = 1
			projectHooks = true
			if hook.Active != c.ProjectTrusted {
				return fmt.Errorf("project hook %d active state disagrees with trust", index+1)
			}
			if c.ProjectRoot != "" && !hookPathContains(c.ProjectRoot, hook.Source) {
				return fmt.Errorf("project hook %d source %q is outside project root %q", index+1, hook.Source, c.ProjectRoot)
			}
		}
		if phase < previousPhase {
			return fmt.Errorf("global hook %d follows the project phase", index+1)
		}
		previousPhase = phase
	}
	if (projectHooks || c.ProjectTrusted) && strings.TrimSpace(c.ProjectRoot) == "" {
		return errors.New("project hooks or trust require a project root")
	}
	return nil
}

func canonicalAbsoluteHookPath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && filepath.IsAbs(value) && value == filepath.Clean(value)
}

func hookPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
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
