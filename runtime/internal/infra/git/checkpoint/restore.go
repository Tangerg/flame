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

// Restore checks out the runID snapshot after archiving the currently admitted
// files. It refuses target paths that overlap unarchived material, including
// ignored files, oversized files, and directories that obstruct target files.
// The pre-restore commit preserves the admitted current state for recovery.
// Store and Application guards serialize Runtime writers; external filesystem
// writers do not participate, so this is not a filesystem transaction.
func (s *Store) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if err := s.checkStorageBoundary(ctx, cwd); err != nil {
		return err
	}
	mu := s.treeLockFor(cwd)
	mu.Lock()
	defer mu.Unlock()
	repoMu := s.repoLockFor(sessionID)
	repoMu.Lock()
	defer repoMu.Unlock()
	gitDir := s.gitDir(sessionID, cwd)
	if !repoExists(gitDir) {
		return ErrUnavailable
	}
	matches, err := repositoryMatchesWorkspace(gitDir, cwd)
	if err != nil {
		return err
	}
	if !matches {
		return ErrUnavailable
	}
	if _, err := s.git(ctx, gitDir, cwd, "rev-parse", "-q", "--verify", "refs/tags/"+tagFor(runID)); err != nil {
		return ErrUnavailable
	}
	if err := s.materializeAlternates(ctx, gitDir); err != nil {
		return err
	}
	// Reversibility: capture the pre-restore state as a commit before checkout,
	// but only when there's something to capture (no empty commit otherwise).
	if err := s.stageChanges(ctx, gitDir, cwd); err != nil {
		return err
	}
	shouldCommit, err := s.shouldCommit(ctx, gitDir, cwd)
	if err != nil {
		return err
	}
	if shouldCommit {
		// The pre-restore commit is what makes the restore reversible (unrevert).
		// If it fails, do NOT proceed to checkout below — that would
		// discard the working-tree state with no recovery point, turning a
		// "reversible" restore irreversible. Fail instead.
		if _, err := s.git(ctx, gitDir, cwd, "commit", "-q", "-m", "pre-restore"); err != nil {
			return err
		}
	}
	if err := s.checkRestoreConflicts(ctx, gitDir, cwd, tagFor(runID)); err != nil {
		return err
	}
	// Checkout also checks for changes after the archive and preflight. Unlike
	// reset --hard (or --merge), --no-overwrite-ignore protects ignored blockers.
	// Once checkout starts, any error conservatively retains the recovery intent.
	if _, err := s.git(ctx, gitDir, cwd, "checkout", "-q", "--detach", "--no-overwrite-ignore", tagFor(runID)); err != nil {
		return fmt.Errorf("%w: %v", ErrRestoreIncomplete, err)
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
		if cause := context.Cause(ctx); cause != nil {
			return cause
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
		if cause := context.Cause(ctx); cause != nil {
			return cause
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
