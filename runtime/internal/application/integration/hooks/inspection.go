package hooks

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
)

// errInvalidInspection reports a resolved Hook catalog that cannot safely
// drive execution or trust management.
var errInvalidInspection = errors.New("hooks: invalid inspection")

// ValidateFor protects the resolved Hook cascade returned for workspaceRoot.
// The installed provider owns order; Runtime admits scope and source,
// including the project boundary used for trust.
func (i Inspection) ValidateFor(workspaceRoot string) error {
	if !canonicalAbsolutePath(workspaceRoot) {
		return fmt.Errorf("%w: workspace root %q is not a canonical absolute path", errInvalidInspection, workspaceRoot)
	}
	if err := hookdata.ValidateHookCascade(len(i.Hooks)); err != nil {
		return fmt.Errorf("%w: %w", errInvalidInspection, err)
	}
	if i.ProjectRoot != "" {
		if !canonicalAbsolutePath(i.ProjectRoot) {
			return fmt.Errorf("%w: project root %q is not a canonical absolute path", errInvalidInspection, i.ProjectRoot)
		}
		if !pathContains(i.ProjectRoot, workspaceRoot) {
			return fmt.Errorf("%w: project root %q does not contain workspace %q", errInvalidInspection, i.ProjectRoot, workspaceRoot)
		}
	} else if i.ProjectTrusted {
		return fmt.Errorf("%w: trusted project has no root", errInvalidInspection)
	}

	for index, hook := range i.Hooks {
		if err := hook.Validate(); err != nil {
			return fmt.Errorf("%w: hook %d: %w", errInvalidInspection, index+1, err)
		}
		if !canonicalAbsolutePath(hook.Source) {
			return fmt.Errorf("%w: hook %d source %q is not a canonical absolute path", errInvalidInspection, index+1, hook.Source)
		}
		switch hook.Scope {
		case hookdata.ScopeGlobal:
		case hookdata.ScopeProject:
			if i.ProjectRoot == "" {
				return fmt.Errorf("%w: project hook %d has no project root", errInvalidInspection, index+1)
			}
			if !pathContains(i.ProjectRoot, hook.Source) {
				return fmt.Errorf("%w: project hook %d source %q is outside project root %q", errInvalidInspection, index+1, hook.Source, i.ProjectRoot)
			}
		default:
			return fmt.Errorf("%w: hook %d has unknown scope %q", errInvalidInspection, index+1, hook.Scope)
		}

	}
	return nil
}

func canonicalAbsolutePath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && filepath.IsAbs(value) && value == filepath.Clean(value)
}

func pathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
