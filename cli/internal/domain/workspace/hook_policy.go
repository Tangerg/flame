package workspace

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

type LifecycleHook struct {
	Event         protocol.HookEvent
	Matcher       string
	Command       string
	Inject        string
	TimeoutMillis int
	Scope         protocol.HookScope
	Source        string
	Active        bool
}

func (h LifecycleHook) Validate() error {
	if err := (protocol.HookInfo{
		Event: h.Event, Matcher: h.Matcher, Command: h.Command, Inject: h.Inject,
		TimeoutMillis: h.TimeoutMillis, Scope: h.Scope, Source: h.Source, Active: h.Active,
	}).ValidateWire(); err != nil {
		return err
	}
	if h.Matcher != "" {
		if _, err := path.Match(h.Matcher, ""); err != nil {
			return fmt.Errorf("hook matcher %q is invalid: %w", h.Matcher, err)
		}
	}
	return nil
}

type HookCatalog struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []LifecycleHook
}

func (c HookCatalog) Validate() error {
	projectHooks := false
	for index, hook := range c.Hooks {
		if err := hook.Validate(); err != nil {
			return fmt.Errorf("hook %d: %w", index+1, err)
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
