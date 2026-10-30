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

	"github.com/Tangerg/flame/qimu/cancellation"
	workspacegit "github.com/Tangerg/flame/runtime/internal/infra/git"
)

// maxCheckpointFileSize caps a single file the checkpoint will stage (2 MiB
// guard). A large unignored binary — a dataset, a built artifact a project
// forgot to .gitignore — would otherwise bloat every snapshot and the shadow
// repo. Oversize files are left out; restore refuses a target that would
// overwrite their current material.
const maxCheckpointFileSize = 2 << 20

const (
	maxCheckpointPaths     = 20_000
	maxCheckpointBytes     = 512 << 20
	gitPathRecordSeparator = "\x00" // Git's -z output protocol.
)

// Snapshot anchors the current state of cwd at the runID boundary: it stages
// the run's changed files (honoring .gitignore + a backstop exclude list +
// the size cap) and tags the boundary by run id. A run that changed nothing
// re-tags the existing HEAD instead of minting an empty commit, so a no-change
// Run costs one ref and zero objects. Idempotent per Run (the tag is moved).
func (s *Store) Snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	if err := s.checkStorageBoundary(ctx, cwd); err != nil {
		return err
	}
	mu := s.treeLockFor(cwd)
	mu.Lock()
	defer mu.Unlock()
	repoMu := s.repoLockFor(sessionID)
	repoMu.Lock()
	defer repoMu.Unlock()
	gitDir, err := s.ensureRepo(ctx, sessionID, cwd)
	if err != nil {
		return err
	}
	if stageChangesErr := s.stageChanges(ctx, gitDir, cwd); stageChangesErr != nil {
		return stageChangesErr
	}
	// Commit only when the staged tree actually differs from HEAD (or there is
	// no HEAD yet — the baseline). A no-change run skips the commit and just
	// re-points its tag at HEAD, so it adds a ref but no empty commit object.
	shouldCommit, err := s.shouldCommit(ctx, gitDir, cwd)
	if err != nil {
		return err
	}
	if shouldCommit {
		if _, err := s.git(ctx, gitDir, cwd, "commit", "-q", "--allow-empty", "-m", "run "+runID); err != nil {
			return err
		}
	}
	if err := s.materializeAlternates(ctx, gitDir); err != nil {
		return err
	}
	if _, err := s.git(ctx, gitDir, cwd, "tag", "-f", tagFor(runID)); err != nil {
		return err
	}
	return nil
}

// shouldCommit reports whether the staged index warrants a new commit. The
// first snapshot (no HEAD yet) always commits the baseline; afterwards a commit
// is made only when the staged tree differs from HEAD — so a no-change run
// re-tags the existing HEAD rather than minting an empty commit.
func (s *Store) shouldCommit(ctx context.Context, gitDir, cwd string) (bool, error) {
	hasHead, err := s.hasHead(ctx, gitDir)
	if err != nil {
		return false, err
	}
	if !hasHead {
		return true, nil // no HEAD → the baseline commit
	}
	// `diff --cached --quiet HEAD` exits non-zero exactly when the staged tree
	// differs from HEAD. Exit 1 means a diff; every other failure is operational
	// and must be reported rather than disguised as a change.
	_, err = s.git(ctx, gitDir, cwd, "diff", "--cached", "--quiet", "HEAD")
	if err == nil {
		return false, nil
	}
	if gitExitCode(err) == 1 {
		return true, nil
	}
	return false, err
}

func (s *Store) hasHead(ctx context.Context, gitDir string) (bool, error) {
	_, err := s.git(ctx, gitDir, "", "rev-parse", "-q", "--verify", "HEAD")
	if err == nil {
		return true, nil
	}
	if gitExitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

// The source repository owns tracked/ignored admission. The shadow index holds
// only its workspace-relative projection; it cannot keep owning a path that the
// source now excludes. Force-add applies only to the admitted material.
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
		if err := cancellation.Err(ctx); err != nil {
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
		// Missing paths leave the projection; only previously captured entries
		// need index removal. Other inspection failures cannot imply absence.
		info, err := os.Lstat(filepath.Join(cwd, p))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
				continue
			}
			return nil, fmt.Errorf("checkpoint: inspect %q: %w", p, err)
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if info.Size() > maxCheckpointFileSize {
			// Crossing the cap retires snapshot ownership without changing the file.
			continue
		}
		if info.Size() > maxCheckpointBytes-admittedBytes {
			return nil, fmt.Errorf(
				"%w: selected file material exceeds %d bytes",
				errSnapshotTooLarge, maxCheckpointBytes,
			)
		}
		admittedBytes += info.Size()
		stage = append(stage, p)
	}
	return stage, nil
}

func (s *Store) updateIndex(ctx context.Context, gitDir, cwd string, prefix, paths []string) error {
	// Update in chunks so a large change set cannot overflow the arg limit.
	const chunk = 256
	for i := 0; i < len(paths); i += chunk {
		args := append(slices.Clone(prefix), paths[i:min(i+chunk, len(paths))]...)
		if _, err := s.git(ctx, gitDir, cwd, args...); err != nil {
			return err
		}
	}
	return nil
}
