package git

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// diffSources runs the tracked-changes git diff for the mode and returns the
// patch text plus the untracked file list (worktree mode only).
func diffSources(ctx context.Context, dir, relPath string, mode Mode) (patch []byte, untracked []string, err error) {
	repository, err := IsRepo(ctx, dir)
	if err != nil {
		return nil, nil, err
	}
	if !repository {
		return nil, nil, ErrNotRepo
	}

	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "-M", "--relative"}
	switch mode {
	case Base:
		base, berr := mergeBase(ctx, dir)
		if berr != nil {
			return nil, nil, berr
		}
		args = append(args, base)
	default: // Worktree
		head, headErr := runAllowingExitCode(ctx, dir, 1, "rev-parse", "--verify", "--quiet", "HEAD")
		if headErr != nil {
			return nil, nil, headErr
		}
		if len(bytes.TrimSpace(head)) == 0 {
			untracked, err = untrackedPaths(ctx, dir, relPath)
			return nil, untracked, err
		}
		args = append(args, "HEAD")
	}
	scopePath, err := gitPathRelativeToWorkspace(dir, relPath)
	if err != nil {
		return nil, nil, err
	}
	args = append(args, "--", scopePath)
	patch, err = run(ctx, dir, args...)
	if err != nil {
		return nil, nil, err
	}

	if mode == Worktree {
		untracked, err = untrackedPaths(ctx, dir, relPath)
		if err != nil {
			return nil, nil, err
		}
	}
	return patch, untracked, nil
}

func gitPathRelativeToWorkspace(dir, path string) (string, error) {
	if path == "" {
		return ".", nil
	}
	if !filepath.IsAbs(path) {
		relative := filepath.Clean(path)
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("git: path %q is outside workspace %q", path, dir)
		}
		return filepath.ToSlash(relative), nil
	}
	relative, err := filepath.Rel(dir, path)
	if err != nil {
		return "", fmt.Errorf("git: resolve workspace-relative path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("git: path %q is outside workspace %q", path, dir)
	}
	return filepath.ToSlash(relative), nil
}

// mergeBase resolves the merge-base of HEAD with the default branch.
func mergeBase(ctx context.Context, dir string) (string, error) {
	baseTip, err := defaultBranchTip(ctx, dir)
	if err != nil {
		return "", err
	}
	head, err := runAllowingExitCode(ctx, dir, 1, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(head)) == 0 {
		return "", ErrNoBase
	}
	// Status 1 means the histories have no common ancestor. Invalid objects,
	// process failures, and cancellation retain their distinct error causes.
	out, err := runAllowingExitCode(ctx, dir, 1, "merge-base", strings.TrimSpace(string(head)), baseTip)
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(string(out))
	if base == "" {
		return "", ErrNoBase
	}
	return base, nil
}

// defaultBranchTip resolves the selected branch tip: origin/HEAD → main → master.
// Full refs prevent Git's short-name lookup from selecting a shadowing tag or
// local branch instead of the chosen base.
func defaultBranchTip(ctx context.Context, dir string) (string, error) {
	out, err := runAllowingExitCode(ctx, dir, 1, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	refs := []string{"refs/heads/main", "refs/heads/master"}
	if ref := strings.TrimSpace(string(out)); strings.HasPrefix(ref, "refs/remotes/") {
		refs = []string{ref}
	}
	for _, ref := range refs {
		out, err := runAllowingExitCode(ctx, dir, 1, "rev-parse", "--verify", "--quiet", ref)
		if err != nil {
			return "", err
		}
		if tip := strings.TrimSpace(string(out)); tip != "" {
			return tip, nil
		}
	}
	return "", ErrNoBase
}
