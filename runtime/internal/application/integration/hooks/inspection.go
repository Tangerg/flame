package hooks

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
)

var errInvalidInspection = errors.New("hooks: invalid inspection")

func (i Inspection) ValidateFor(workspaceRoot string) error {
	if !canonicalAbsolutePath(workspaceRoot) {
		return fmt.Errorf("%w: workspace root %q is not a canonical absolute path", errInvalidInspection, workspaceRoot)
	}
	if err := hookdata.ValidateHookCascade(len(i.Hooks)); err != nil {
		return fmt.Errorf("%w: %w", errInvalidInspection, err)
	}
	if i.RepositoryRoot != "" {
		if !canonicalAbsolutePath(i.RepositoryRoot) {
			return fmt.Errorf("%w: repository root %q is not a canonical absolute path", errInvalidInspection, i.RepositoryRoot)
		}
		if !pathContains(i.RepositoryRoot, workspaceRoot) {
			return fmt.Errorf("%w: repository root %q does not contain workspace %q", errInvalidInspection, i.RepositoryRoot, workspaceRoot)
		}
	} else if i.RepositoryTrusted {
		return fmt.Errorf("%w: trusted repository has no root", errInvalidInspection)
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
		case hookdata.ScopeRepository:
			if i.RepositoryRoot == "" {
				return fmt.Errorf("%w: repository hook %d has no repository root", errInvalidInspection, index+1)
			}
			if !pathContains(i.RepositoryRoot, hook.Source) {
				return fmt.Errorf("%w: repository hook %d source %q is outside repository root %q", errInvalidInspection, index+1, hook.Source, i.RepositoryRoot)
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
