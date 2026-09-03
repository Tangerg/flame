package skillauthoring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
)

// archiveActive moves the active skill <name> into _archive/<name>, OVERWRITING
// any older archived version — the single history slot the module keeps. The
// caller holds s.mu and owns root. Shared by the revision-replace path and the
// idle-lifecycle sweep.
func (s *Store) archiveActive(root *os.Root, name string) ([]string, error) {
	activeDir := s.activeDir(name)
	content, found, err := readSkill(root, activeDir)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: cannot archive %q", skills.ErrNotFound, name)
	}
	archiveDir := s.archiveDir(name)
	removed, err := removeSkillTree(root, archiveDir)
	identities := identitiesIf(removed, s.skillIdentities(archiveDir))
	if err != nil {
		return identities, fmt.Errorf("skillauthoring: clear archive slot for %q: %w", name, err)
	}
	if err := root.MkdirAll(archivedSubdir, 0o755); err != nil {
		return identities, fmt.Errorf("skillauthoring: create archive area: %w", err)
	}
	if err := root.Rename(activeDir, archiveDir); err != nil {
		moved, reconcileErr := reconcileLifecycleRename(root, name, activeDir, archiveDir, "archive", content, err)
		if moved {
			return distinctPaths(append(identities, s.skillIdentities(activeDir, archiveDir)...)), reconcileErr
		}
		return identities, reconcileErr
	}
	return distinctPaths(append(identities, s.skillIdentities(activeDir, archiveDir)...)), nil
}

// Archive moves an active skill out of discovery without deleting it, returns
// the exact public file identities changed by the move, and drops
// its usage record. Dropping the record — the same thing the idle sweep does on
// auto-archive — makes "a restored skill starts with a fresh grace floor" hold
// no matter which path archived it: without it, a manually archived-then-restored
// agent-authored skill would carry a stale last-used time and be re-archived on
// the next sweep.
func (s *Store) Archive(ctx context.Context, name string) ([]string, error) {
	identities, err := s.moveLifecycle(ctx, name, skills.Active, skills.Archived)
	if err != nil {
		return identities, err
	}
	return identities, s.dropUsage(ctx, name)
}

// Restore moves an archived skill back into the active set and returns the
// exact public file identities changed by the move.
func (s *Store) Restore(ctx context.Context, name string) ([]string, error) {
	// Drop any leftover usage record BEFORE the move so the restored skill always
	// starts with a fresh grace floor — even if an earlier Archive crashed between
	// its rename and its own dropUsage, leaving a stale record. move + dropUsage
	// are two filesystem operations and cannot be atomic; dropping first makes a
	// crash here either a no-op re-restore (still archived, usage already gone) or
	// a clean fresh floor (moved, usage already gone), never active-with-stale-usage.
	if err := s.dropUsage(ctx, name); err != nil {
		return nil, err
	}
	return s.moveLifecycle(ctx, name, skills.Archived, skills.Active)
}

func (s *Store) moveLifecycle(ctx context.Context, name string, from, to skills.Lifecycle) ([]string, error) {
	if !s.Enabled() {
		return nil, errors.New("skillauthoring: no skills root configured")
	}
	if !validName(name) {
		return nil, fmt.Errorf("skillauthoring: invalid skill name %q", name)
	}
	operation, err := lifecycleOperation(from, to)
	if err != nil {
		return nil, err
	}
	source, err := s.lifecycleDir(from, name)
	if err != nil {
		return nil, err
	}
	destination, err := s.lifecycleDir(to, name)
	if err != nil {
		return nil, err
	}
	if contextErrorErr := contextError(ctx, operation+" skill"); contextErrorErr != nil {
		return nil, contextErrorErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, operation+" skill")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	info, err := root.Lstat(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, inspectCompletedLifecycleMove(root, name, destination, operation)
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: cannot %s %q: %w", operation, name, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skillauthoring: cannot %s %q: source is not a directory", operation, name)
	}
	content, found, err := readSkill(root, source)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: cannot %s %q", skills.ErrNotFound, operation, name)
	}
	if err := validateSkill(name, content); err != nil {
		return nil, err
	}
	if _, err := root.Lstat(destination); err == nil {
		return nil, fmt.Errorf("%w: cannot %s %q", skills.ErrConflict, operation, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("skillauthoring: inspect %s destination for %q: %w", operation, name, err)
	}
	if err := root.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return nil, fmt.Errorf("skillauthoring: prepare %s destination for %q: %w", operation, name, err)
	}
	if err := contextError(ctx, operation+" skill"); err != nil {
		return nil, err
	}
	if err := root.Rename(source, destination); err != nil {
		moved, reconcileErr := reconcileLifecycleRename(root, name, source, destination, operation, content, err)
		if moved {
			return s.skillIdentities(source, destination), reconcileErr
		}
		return nil, reconcileErr
	}
	return s.skillIdentities(source, destination), nil
}

func inspectCompletedLifecycleMove(root *os.Root, name, destination, operation string) error {
	content, found, err := readSkill(root, destination)
	if err != nil {
		return fmt.Errorf("skillauthoring: inspect completed %s for %q: %w", operation, name, err)
	}
	if found {
		if err := validateSkill(name, content); err != nil {
			return fmt.Errorf("%w: cannot replay %s %q: %w", skills.ErrConflict, operation, name, err)
		}
		return nil
	}
	if _, err := root.Lstat(destination); err == nil {
		return fmt.Errorf(
			"%w: cannot replay %s %q: destination is not a valid skill",
			skills.ErrConflict,
			operation,
			name,
		)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("skillauthoring: inspect %s destination for %q: %w", operation, name, err)
	}
	return fmt.Errorf("%w: cannot %s %q", skills.ErrNotFound, operation, name)
}

func reconcileLifecycleRename(
	root *os.Root,
	name string,
	source string,
	destination string,
	operation string,
	content []byte,
	renameErr error,
) (bool, error) {
	moved, found, readErr := readSkill(root, destination)
	if readErr != nil {
		return false, fmt.Errorf(
			"skillauthoring: inspect %s outcome for %q: %w",
			operation,
			name,
			errors.Join(renameErr, readErr),
		)
	}
	if found && bytes.Equal(moved, content) {
		if _, sourceErr := root.Lstat(source); errors.Is(sourceErr, fs.ErrNotExist) {
			return true, nil
		} else if sourceErr != nil {
			return false, fmt.Errorf(
				"skillauthoring: inspect %s source for %q: %w",
				operation,
				name,
				sourceErr,
			)
		}
	}
	if _, err := root.Lstat(destination); err == nil {
		return false, fmt.Errorf("%w: cannot %s %q", skills.ErrConflict, operation, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf(
			"skillauthoring: inspect %s destination for %q: %w",
			operation,
			name,
			errors.Join(renameErr, err),
		)
	}
	return false, fmt.Errorf("skillauthoring: %s %q: %w", operation, name, renameErr)
}

func lifecycleOperation(from, to skills.Lifecycle) (string, error) {
	switch {
	case from == skills.Active && to == skills.Archived:
		return "archive", nil
	case from == skills.Archived && to == skills.Active:
		return "restore", nil
	default:
		return "", fmt.Errorf("skillauthoring: unsupported lifecycle transition %q to %q", from, to)
	}
}
