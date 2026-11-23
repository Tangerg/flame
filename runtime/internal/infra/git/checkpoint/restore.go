package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if err := s.checkStorageBoundary(ctx, cwd); err != nil {
		return err
	}
	release, err := s.acquireWorkspace(ctx, sessionID, cwd)
	if err != nil {
		return err
	}
	defer release()
	gitDir := s.gitDir(sessionID, cwd)
	if err := s.requireBoundary(ctx, gitDir, cwd, runID); err != nil {
		return err
	}
	if err := s.materializeAlternates(ctx, gitDir); err != nil {
		return err
	}
	// The pre-restore commit is what makes the restore reversible. If it fails,
	// checkout must not run: it would discard the working tree with no recovery
	// point.
	if err := s.archiveWorkspace(ctx, gitDir, cwd, "pre-restore"); err != nil {
		return err
	}
	if err := s.checkRestoreConflicts(ctx, gitDir, cwd, tagFor(runID)); err != nil {
		return err
	}
	// Checkout also checks for changes after the archive and preflight. Unlike
	// reset --hard (or --merge), --no-overwrite-ignore protects ignored blockers.
	// Once checkout starts, any error conservatively retains the recovery intent.
	if err := s.git(ctx, gitDir, cwd, "checkout", "-q", "--detach", "--no-overwrite-ignore", tagFor(runID)); err != nil {
		return fmt.Errorf("%w: %w", ErrRestoreIncomplete, err)
	}
	return nil
}

func (s *Store) requireBoundary(ctx context.Context, gitDir, cwd, runID string) error {
	present, err := repoExists(gitDir)
	if err != nil {
		return err
	}
	if !present {
		return ErrUnavailable
	}
	matches, err := repositoryMatchesWorkspace(gitDir, cwd)
	if err != nil {
		return err
	}
	if !matches {
		return ErrUnavailable
	}
	if err := s.git(ctx, gitDir, cwd, "rev-parse", "-q", "--verify", "refs/tags/"+tagFor(runID)); err != nil {
		if gitExitCode(err) == 1 {
			return ErrUnavailable
		}
		return err
	}
	return nil
}

func (s *Store) checkRestoreConflicts(ctx context.Context, gitDir, cwd, target string) error {
	output, err := s.gitOutput(ctx, gitDir, cwd, "ls-tree", "-r", "-z", "--name-only", target)
	if err != nil {
		return err
	}
	targets, err := restorePaths(output)
	if err != nil {
		return err
	}
	output, err = s.gitOutput(ctx, gitDir, cwd, "ls-files", "-z", "--cached")
	if err != nil {
		return err
	}
	paths, err := restorePaths(output)
	if err != nil {
		return err
	}
	archived := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		archived[path] = struct{}{}
	}
	root, err := os.OpenRoot(cwd)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for _, target := range targets {
		if err := checkTargetOwnership(ctx, root, target, archived); err != nil {
			return err
		}
	}
	return nil
}

func checkTargetOwnership(ctx context.Context, root *os.Root, target string, archived map[string]struct{}) error {
	var path string
	for component := range strings.SplitSeq(target, "/") {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return err
		}
		if path != "" {
			path += "/"
		}
		path += component
		info, err := root.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checkpoint: inspect restore path %q: %w", path, err)
		}
		if !info.IsDir() {
			return requireArchived(path, info.Mode(), archived)
		}
	}
	// Git's untracked listing omits special nodes such as FIFOs. Inspect the
	// actual obstructing subtree without following symlinks before permitting
	// checkout to replace a directory with a target file.
	return fs.WalkDir(root.FS(), target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("checkpoint: inspect restore subtree %q: %w", path, err)
		}
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		return requireArchived(path, entry.Type(), archived)
	})
}

func requireArchived(path string, mode fs.FileMode, archived map[string]struct{}) error {
	_, captured := archived[path]
	if !captured || (!mode.IsRegular() && mode&os.ModeSymlink == 0) {
		return fmt.Errorf("%w: %q", ErrConflict, path)
	}
	return nil
}

func restorePaths(output []byte) ([]string, error) {
	if len(output) == 0 {
		return nil, nil
	}
	if output[len(output)-1] != 0 {
		return nil, errors.New("checkpoint: git returned an incomplete path record")
	}
	paths := strings.Split(string(output[:len(output)-1]), gitPathRecordSeparator)
	for _, path := range paths {
		if !filepath.IsLocal(path) {
			return nil, fmt.Errorf("checkpoint: git returned non-local path %q", path)
		}
	}
	return paths, nil
}
