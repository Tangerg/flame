package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	workspacegit "github.com/Tangerg/flame/runtime/internal/infra/git"
)

const maxCheckpointFileSize = 2 << 20

const (
	maxCheckpointPaths     = 20_000
	maxCheckpointBytes     = 512 << 20
	gitPathRecordSeparator = "\x00"
)

func (s *Store) Snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	if err := s.checkStorageBoundary(ctx, cwd); err != nil {
		return err
	}
	release, err := s.acquireWorkspace(ctx, sessionID, cwd)
	if err != nil {
		return err
	}
	defer release()
	gitDir, err := s.ensureRepo(ctx, sessionID, cwd)
	if err != nil {
		return err
	}
	if err := s.archiveWorkspace(ctx, gitDir, cwd, "run "+runID); err != nil {
		return err
	}
	if err := s.materializeAlternates(ctx, gitDir); err != nil {
		return err
	}
	if err := s.git(ctx, gitDir, cwd, "tag", "-f", tagFor(runID)); err != nil {
		return err
	}
	return nil
}

func (s *Store) archiveWorkspace(ctx context.Context, gitDir, cwd, message string) error {
	if err := s.stageChanges(ctx, gitDir, cwd); err != nil {
		return err
	}
	shouldCommit, err := s.shouldCommit(ctx, gitDir, cwd)
	if err != nil || !shouldCommit {
		return err
	}
	err = s.git(ctx, gitDir, cwd, "commit", "-q", "--allow-empty", "-m", message)
	return err
}

func (s *Store) shouldCommit(ctx context.Context, gitDir, cwd string) (bool, error) {
	hasHead, err := s.hasHead(ctx, gitDir)
	if err != nil {
		return false, err
	}
	if !hasHead {
		return true, nil
	}
	// `diff --cached --quiet HEAD` exits non-zero exactly when the staged tree
	// differs from HEAD. Exit 1 means a diff; every other failure is operational
	// and must be reported rather than disguised as a change.
	err = s.git(ctx, gitDir, cwd, "diff", "--cached", "--quiet", "HEAD")
	if err == nil {
		return false, nil
	}
	if gitExitCode(err) == 1 {
		return true, nil
	}
	return false, err
}

func (s *Store) hasHead(ctx context.Context, gitDir string) (bool, error) {
	err := s.git(ctx, gitDir, "", "rev-parse", "-q", "--verify", "HEAD")
	if err == nil {
		return true, nil
	}
	if gitExitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

func (s *Store) stageChanges(ctx context.Context, gitDir, cwd string) error {
	output, err := s.snapshotCandidates(ctx, gitDir, cwd)
	if err != nil {
		return err
	}
	stage, err := checkpointCandidates(ctx, cwd, output)
	if err != nil {
		return err
	}
	output, err = s.gitOutput(ctx, gitDir, cwd, "ls-files", "-z", "--cached")
	if err != nil {
		return err
	}
	previous, err := restorePaths(output)
	if err != nil {
		return err
	}
	selected := make(map[string]struct{}, len(stage))
	for _, path := range stage {
		selected[path] = struct{}{}
	}
	var untrack []string
	for _, path := range previous {
		if _, present := selected[path]; !present {
			untrack = append(untrack, path)
		}
	}
	if err := s.updateIndex(ctx, gitDir, cwd,
		[]string{"rm", "-q", "-r", "-f", "--cached", "--ignore-unmatch", "--"}, untrack); err != nil {
		return err
	}
	return s.updateIndex(ctx, gitDir, cwd, []string{"add", "--force", "--"}, stage)
}

func (s *Store) snapshotCandidates(ctx context.Context, gitDir, cwd string) ([]byte, error) {
	repository, err := workspacegit.IsRepo(ctx, cwd)
	if err != nil {
		return nil, err
	}
	args := []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"}
	if repository {
		for pattern := range strings.SplitSeq(strings.TrimSpace(commonExcludes), "\n") {
			args = append(args, "--exclude="+pattern)
		}
		// Without the pathspec Git also emits ancestor/sibling index entries.
		return gitInOutput(ctx, cwd, append(args, "--", ".")...)
	}
	return s.gitOutput(ctx, gitDir, cwd, args...)
}

func checkpointCandidates(ctx context.Context, cwd string, output []byte) ([]string, error) {
	if len(output) > 0 && output[len(output)-1] != 0 {
		return nil, errors.New("checkpoint: git ls-files returned an incomplete record")
	}
	var stage []string
	seen := make(map[string]struct{}, min(maxCheckpointPaths, len(output)/2))
	var admittedBytes int64
	for p := range strings.SplitSeq(string(output), gitPathRecordSeparator) {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			return nil, err
		}
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		if len(seen) == maxCheckpointPaths {
			return nil, fmt.Errorf(
				"%w: more than %d selected paths",
				errSnapshotTooLarge, maxCheckpointPaths,
			)
		}
		if !filepath.IsLocal(p) {
			return nil, fmt.Errorf("checkpoint: git returned non-local path %q", p)
		}
		seen[p] = struct{}{}
		size, admitted, err := candidateSize(cwd, p)
		if err != nil {
			return nil, err
		}
		if !admitted {
			continue
		}
		if size > maxCheckpointBytes-admittedBytes {
			return nil, fmt.Errorf(
				"%w: selected file material exceeds %d bytes",
				errSnapshotTooLarge, maxCheckpointBytes,
			)
		}
		admittedBytes += size
		stage = append(stage, p)
	}
	return stage, nil
}

func candidateSize(cwd, path string) (int64, bool, error) {
	info, err := os.Lstat(filepath.Join(cwd, path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("checkpoint: inspect %q: %w", path, err)
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return 0, false, nil
	}
	if info.Size() > maxCheckpointFileSize {
		return 0, false, nil
	}
	return info.Size(), true, nil
}

func (s *Store) updateIndex(ctx context.Context, gitDir, cwd string, prefix, paths []string) error {
	// Update in chunks so a large change set cannot overflow the arg limit.
	const chunk = 256
	for i := 0; i < len(paths); i += chunk {
		args := append(slices.Clone(prefix), paths[i:min(i+chunk, len(paths))]...)
		if err := s.git(ctx, gitDir, cwd, args...); err != nil {
			return err
		}
	}
	return nil
}
