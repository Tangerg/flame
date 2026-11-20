package skillauthoring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
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
		moved, reconcileErr := reconcileLifecycleRename(root, name, activeDir, archiveDir, archiveMove.verb, content, err)
		if moved {
			return distinctPaths(append(identities, s.skillIdentities(activeDir, archiveDir)...)), reconcileErr
		}
		return identities, reconcileErr
	}
	return distinctPaths(append(identities, s.skillIdentities(activeDir, archiveDir)...)), nil
}

// lifecycleMove is one of the two Skill relocations the store performs.
type lifecycleMove struct {
	verb     string
	from, to skills.Lifecycle
}

var (
	archiveMove = lifecycleMove{verb: "archive", from: skills.Active, to: skills.Archived}
	restoreMove = lifecycleMove{verb: "restore", from: skills.Archived, to: skills.Active}
)

// Archive moves an active skill out of discovery and retires its usage under
// one library lease. Returned identities include any committed move on error.
func (s *Store) Archive(ctx context.Context, name string) ([]string, error) {
	return s.moveLifecycle(ctx, name, archiveMove)
}

// Restore moves an archived skill back into the active set and returns the
// exact public file identities changed by the move.
func (s *Store) Restore(ctx context.Context, name string) ([]string, error) {
	return s.moveLifecycle(ctx, name, restoreMove)
}

func (s *Store) lifecyclePaths(name string, move lifecycleMove) (string, string, error) {
	if !validName(name) {
		return "", "", fmt.Errorf("skillauthoring: invalid skill name %q", name)
	}
	source, err := s.lifecycleDir(move.from, name)
	if err != nil {
		return "", "", err
	}
	destination, err := s.lifecycleDir(move.to, name)
	return source, destination, err
}

func (s *Store) moveLifecycle(ctx context.Context, name string, move lifecycleMove) ([]string, error) {
	source, destination, err := s.lifecyclePaths(name, move)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx, move.verb+" skill"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, move.verb+" skill")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	content, replayed, err := readMoveSource(ctx, root, name, source, destination, move)
	if err != nil || replayed {
		return nil, err
	}
	if err := prepareMoveDestination(root, name, destination, move); err != nil {
		return nil, err
	}
	if err := contextError(ctx, move.verb+" skill"); err != nil {
		return nil, err
	}
	usage, err := readUsage(ctx, root)
	if err != nil {
		return nil, err
	}
	_, hadUsage := usage[name]
	delete(usage, name)
	// Reset archived activity before restoring visibility so a crash cannot
	// expose an active Skill with a stale grace floor. Replays returned above
	// without advancing either lifecycle or activity.
	if move == restoreMove && hadUsage {
		if err := writeUsage(ctx, root, usage); err != nil {
			return nil, err
		}
	}
	if err := renameLifecycle(root, name, source, destination, move.verb, content); err != nil {
		return nil, err
	}
	identities := s.skillIdentities(source, destination)
	if move == archiveMove && hadUsage {
		return identities, writeUsage(ctx, root, usage)
	}
	return identities, nil
}

// renameLifecycle moves the skill, accepting a rename that already landed
// before an interrupted attempt could report it.
func renameLifecycle(root *os.Root, name, source, destination, verb string, content []byte) error {
	err := root.Rename(source, destination)
	if err == nil {
		return nil
	}
	if moved, reconcileErr := reconcileLifecycleRename(root, name, source, destination, verb, content, err); !moved {
		return reconcileErr
	}
	return nil
}

// readMoveSource returns the valid Skill about to move. A missing source is a
// replay of a move that already committed, settled from its destination.
func readMoveSource(ctx context.Context, root *os.Root, name, source, destination string, move lifecycleMove) ([]byte, bool, error) {
	info, err := root.Lstat(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, true, inspectCompletedLifecycleMove(ctx, root, name, destination, move.verb)
	}
	if err != nil {
		return nil, false, fmt.Errorf("skillauthoring: cannot %s %q: %w", move.verb, name, err)
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("skillauthoring: cannot %s %q: source is not a directory", move.verb, name)
	}
	content, found, err := readSkill(root, source)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, fmt.Errorf("%w: cannot %s %q", skills.ErrNotFound, move.verb, name)
	}
	if _, err := validateSkill(ctx, name, content); err != nil {
		return nil, false, err
	}
	return content, false, nil
}

func prepareMoveDestination(root *os.Root, name, destination string, move lifecycleMove) error {
	if _, err := root.Lstat(destination); err == nil {
		return fmt.Errorf("%w: cannot %s %q", skills.ErrConflict, move.verb, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("skillauthoring: inspect %s destination for %q: %w", move.verb, name, err)
	}
	if err := root.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("skillauthoring: prepare %s destination for %q: %w", move.verb, name, err)
	}
	return nil
}

func inspectCompletedLifecycleMove(ctx context.Context, root *os.Root, name, destination, operation string) error {
	content, found, err := readSkill(root, destination)
	if err != nil {
		return fmt.Errorf("skillauthoring: inspect completed %s for %q: %w", operation, name, err)
	}
	if found {
		if _, err := validateSkill(ctx, name, content); err != nil {
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
