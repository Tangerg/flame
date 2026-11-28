package skillauthoring

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

type LifecycleMove struct {
	Name             string
	Destination      skills.Lifecycle
	DirectoryID      string
	DocumentRevision string
	Receipt          operation.ReplayClaim
}

type MoveJournal interface {
	Prepare(context.Context, LifecycleMove) error
	Pending(context.Context) (LifecycleMove, bool, error)
	Complete(context.Context, LifecycleMove, error) error
}

var errUserLifecycleRequired = errors.New("skillauthoring: lifecycle commands require the personal library")

func (s *Store) Move(ctx context.Context, name string, to skills.Lifecycle, receipt operation.ReplayClaim) ([]string, error) {
	if s.moves == nil {
		return nil, errUserLifecycleRequired
	}
	if err := skills.ValidateName(name); err != nil {
		return nil, err
	}
	if err := to.Validate(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, release, err := s.openWritableRoot(ctx, "move skill")
	if err != nil {
		return nil, err
	}
	defer release()
	source, destination := s.movePaths(name, to)
	directory, revision, err := readMoveSource(ctx, root, name, source)
	if err != nil {
		return nil, err
	}
	if err := requireVacantDirectory(root, destination); err != nil {
		return nil, err
	}
	usage, err := readUsage(ctx, root)
	if err != nil {
		return nil, err
	}
	move := LifecycleMove{Name: name, Destination: to, DirectoryID: directory, DocumentRevision: revision, Receipt: receipt}
	if err := s.moves.Prepare(ctx, move); err != nil {
		return nil, err
	}
	moved, refusal, err := s.completeMove(ctx, root, move, usage)
	return moved, errors.Join(refusal, err)
}

func (s *Store) RecoverPendingMove(ctx context.Context) ([]string, error) {
	if s.moves == nil {
		return nil, errUserLifecycleRequired
	}
	move, pending, err := s.moves.Pending(ctx)
	if err != nil || !pending {
		return nil, err
	}
	return s.ResumeMove(ctx, move.Receipt)
}

func (s *Store) ResumeMove(ctx context.Context, receipt operation.ReplayClaim) ([]string, error) {
	if s.moves == nil {
		return nil, errUserLifecycleRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, release, err := s.openLeasedRoot(ctx, "recover skill move")
	if err != nil {
		return nil, err
	}
	defer release()
	move, pending, err := s.moves.Pending(ctx)
	if err != nil || !pending || move.Receipt != receipt {
		return nil, err
	}
	usage, err := readUsage(ctx, root)
	if err != nil {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	changed, _, err := s.completeMove(ctx, root, move, usage)
	return changed, err
}

func (s *Store) requireSettledMove(ctx context.Context) error {
	if s.moves == nil {
		return nil
	}
	_, pending, err := s.moves.Pending(ctx)
	if err != nil {
		return err
	}
	if pending {
		return operation.ErrInProgress
	}
	return nil
}

func (s *Store) movePaths(name string, to skills.Lifecycle) (string, string) {
	active, archived := s.activeDir(name), s.archiveDir(name)
	if to == skills.Active {
		return archived, active
	}
	return active, archived
}

func readMoveSource(ctx context.Context, root *os.Root, name, source string) (string, string, error) {
	directory, err := moveDirectoryID(root, source)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", fmt.Errorf("%w: %q", skills.ErrNotFound, name)
	}
	if err != nil {
		return "", "", err
	}
	content, found, err := readSkill(root, source)
	if err != nil {
		return "", "", err
	}
	if !found {
		return "", "", fmt.Errorf("%w: %q", skills.ErrNotFound, name)
	}
	if err := validateSkill(ctx, name, content); err != nil {
		return "", "", err
	}
	return directory, digestDocument(content, true), nil
}

func (s *Store) completeMove(ctx context.Context, root *os.Root, move LifecycleMove, usage map[string]usageRecord) (changed []string, refusal, pendingErr error) {
	source, destination := s.movePaths(move.Name, move.Destination)
	moved, cause := move.apply(ctx, root, source, destination, usage)
	if moved {
		changed = s.skillIdentities(source, destination)
	}
	if cause != nil && !errors.Is(cause, skills.ErrNotFound) && !errors.Is(cause, skills.ErrConflict) {
		return changed, nil, errors.Join(operation.ErrOutcomePending, cause)
	}
	if err := s.moves.Complete(ctx, move, cause); err != nil {
		return changed, nil, errors.Join(operation.ErrOutcomePending, err)
	}
	return changed, cause, nil
}

func (m LifecycleMove) apply(ctx context.Context, root *os.Root, source, destination string, usage map[string]usageRecord) (bool, error) {
	current, err := moveDirectoryID(root, destination)
	if err == nil && current == m.DirectoryID {
		return true, m.finishArchive(ctx, root, usage)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, skills.ErrConflict) {
		return false, err
	}
	if sourceErr := m.verifySource(root, source); sourceErr != nil {
		return false, sourceErr
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("%w: %q", skills.ErrConflict, destination)
	}
	if err := root.MkdirAll(filepath.Dir(destination), filesystem.SharedDirectoryMode); err != nil {
		return false, err
	}
	// Give a restored Skill a fresh grace floor before making it discoverable.
	if m.Destination == skills.Active {
		if err := clearSkillUsage(ctx, root, usage, m.Name); err != nil {
			return false, err
		}
	}
	if err := root.Rename(source, destination); err != nil {
		return false, err
	}
	return true, m.finishArchive(ctx, root, usage)
}

func (m LifecycleMove) verifySource(root *os.Root, source string) error {
	directory, err := moveDirectoryID(root, source)
	if err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, skills.ErrConflict) {
		return err
	}
	if directory != m.DirectoryID {
		return fmt.Errorf("%w: original directory for %q", skills.ErrNotFound, m.Name)
	}
	content, found, err := readSkill(root, source)
	if err != nil {
		return err
	}
	if digestDocument(content, found) != m.DocumentRevision {
		return fmt.Errorf("%w: original document for %q", skills.ErrNotFound, m.Name)
	}
	return nil
}

func (m LifecycleMove) finishArchive(ctx context.Context, root *os.Root, usage map[string]usageRecord) error {
	if m.Destination == skills.Archived {
		return clearSkillUsage(ctx, root, usage, m.Name)
	}
	return nil
}

func clearSkillUsage(ctx context.Context, root *os.Root, usage map[string]usageRecord, name string) error {
	if _, exists := usage[name]; !exists {
		return nil
	}
	delete(usage, name)
	return writeUsage(ctx, root, usage)
}

func (s *Store) archiveActive(root *os.Root, name string) ([]string, error) {
	active, archived := s.movePaths(name, skills.Archived)
	if err := requireVacantDirectory(root, archived); err != nil {
		return nil, err
	}
	if err := root.MkdirAll(archivedSubdir, filesystem.SharedDirectoryMode); err != nil {
		return nil, fmt.Errorf("skillauthoring: create archive area: %w", err)
	}
	if err := root.Rename(active, archived); err != nil {
		return nil, fmt.Errorf("skillauthoring: archive %q: %w", name, err)
	}
	return s.skillIdentities(active, archived), nil
}

func requireVacantDirectory(root *os.Root, directory string) error {
	_, err := root.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %q", skills.ErrConflict, directory)
}
