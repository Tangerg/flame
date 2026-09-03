package hooks

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	domain "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

// ErrInvalidInspection reports a resolved Hook catalog that cannot safely
// drive execution or trust management.
var ErrInvalidInspection = errors.New("hooks: invalid inspection")

// ValidateFor protects the resolved Hook cascade returned for workspaceRoot.
// Hook order is executable policy: global entries run before project entries,
// while order within each phase remains the authored loader order.
func (i Inspection) ValidateFor(workspaceRoot string) error {
	if !canonicalAbsolutePath(workspaceRoot) {
		return fmt.Errorf("%w: workspace root %q is not a canonical absolute path", ErrInvalidInspection, workspaceRoot)
	}
	if err := domain.ValidateHookCascade(len(i.Hooks)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInspection, err)
	}
	if i.ProjectRoot != "" {
		if !canonicalAbsolutePath(i.ProjectRoot) {
			return fmt.Errorf("%w: project root %q is not a canonical absolute path", ErrInvalidInspection, i.ProjectRoot)
		}
		if !pathContains(i.ProjectRoot, workspaceRoot) {
			return fmt.Errorf("%w: project root %q does not contain workspace %q", ErrInvalidInspection, i.ProjectRoot, workspaceRoot)
		}
	} else if i.ProjectTrusted {
		return fmt.Errorf("%w: trusted project has no root", ErrInvalidInspection)
	}

	previousPhase := -1
	for index, hook := range i.Hooks {
		if err := hook.Validate(); err != nil {
			return fmt.Errorf("%w: hook %d: %w", ErrInvalidInspection, index+1, err)
		}
		if !canonicalAbsolutePath(hook.Source) {
			return fmt.Errorf("%w: hook %d source %q is not a canonical absolute path", ErrInvalidInspection, index+1, hook.Source)
		}
		phase := 0
		switch hook.Scope {
		case domain.ScopeGlobal:
		case domain.ScopeProject:
			phase = 1
			if i.ProjectRoot == "" {
				return fmt.Errorf("%w: project hook %d has no project root", ErrInvalidInspection, index+1)
			}
			if !pathContains(i.ProjectRoot, hook.Source) {
				return fmt.Errorf("%w: project hook %d source %q is outside project root %q", ErrInvalidInspection, index+1, hook.Source, i.ProjectRoot)
			}
		default:
			return fmt.Errorf("%w: hook %d has unknown scope %q", ErrInvalidInspection, index+1, hook.Scope)
		}
		if phase < previousPhase {
			return fmt.Errorf("%w: global hook %d follows the project phase", ErrInvalidInspection, index+1)
		}
		previousPhase = phase
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
