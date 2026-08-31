package workspace

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

type HookEvent string

const (
	HookPreToolUse       HookEvent = "PreToolUse"
	HookPostToolUse      HookEvent = "PostToolUse"
	HookUserPromptSubmit HookEvent = "UserPromptSubmit"
	HookSessionStart     HookEvent = "SessionStart"
	HookSubagentStart    HookEvent = "SubagentStart"
	HookSubagentStop     HookEvent = "SubagentStop"
	HookPreCompact       HookEvent = "PreCompact"
	HookStop             HookEvent = "Stop"
	HookNotification     HookEvent = "Notification"
)

func (e HookEvent) Validate() error {
	switch e {
	case HookPreToolUse, HookPostToolUse, HookUserPromptSubmit, HookSessionStart, HookSubagentStart, HookSubagentStop, HookPreCompact, HookStop, HookNotification:
		return nil
	default:
		return fmt.Errorf("hook event %q is invalid", e)
	}
}

type HookScope string

const (
	HookGlobal  HookScope = "global"
	HookProject HookScope = "project"
)

func (s HookScope) Validate() error {
	if s != HookGlobal && s != HookProject {
		return fmt.Errorf("hook scope %q is invalid", s)
	}
	return nil
}

type LifecycleHook struct {
	Event         HookEvent
	Matcher       string
	Command       string
	Inject        string
	TimeoutMillis int
	Scope         HookScope
	Source        string
	Active        bool
}

func (h LifecycleHook) Validate() error {
	if err := h.Event.Validate(); err != nil {
		return err
	}
	if err := h.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(h.Source) == "" {
		return errors.New("hook source is empty")
	}
	hasCommand := strings.TrimSpace(h.Command) != ""
	hasInject := strings.TrimSpace(h.Inject) != ""
	if hasCommand == hasInject {
		return errors.New("hook requires exactly one of command or inject")
	}
	if h.TimeoutMillis < 0 || (hasInject && h.TimeoutMillis != 0) {
		return errors.New("hook timeout is invalid")
	}
	if h.Matcher != "" {
		if h.Event != HookPreToolUse && h.Event != HookPostToolUse {
			return errors.New("hook matcher is only valid for tool events")
		}
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
		if hook.Scope == HookGlobal && !hook.Active {
			return fmt.Errorf("global hook %d is inactive", index+1)
		}
		if hook.Scope == HookProject {
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

type HookService interface {
	Catalog(context.Context, string) (HookCatalog, error)
	SetProjectTrust(context.Context, string, bool) error
}
