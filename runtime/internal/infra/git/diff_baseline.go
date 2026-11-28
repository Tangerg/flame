package git

import (
	"bytes"
	"context"
	"strings"
)

func diffBaseline(ctx context.Context, dir string, mode Mode) (Baseline, string, error) {
	repository, err := IsRepo(ctx, dir)
	if err != nil {
		return Baseline{}, "", err
	}
	if !repository {
		return Baseline{}, "", ErrNotRepo
	}
	if mode == Base {
		base, err := mergeBase(ctx, dir)
		if err != nil {
			return Baseline{}, "", err
		}
		return Baseline{Type: BaselineMergeBase, Commit: base}, base, nil
	}
	head, err := runAllowingExitCode(ctx, dir, 1, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return Baseline{}, "", err
	}
	if commit := strings.TrimSpace(string(head)); commit != "" {
		return Baseline{Type: BaselineHead, Commit: commit}, commit, nil
	}
	empty, err := run(ctx, dir, "hash-object", "-t", "tree", "--stdin")
	if err != nil {
		return Baseline{}, "", err
	}
	return Baseline{Type: BaselineEmptyTree}, strings.TrimSpace(string(empty)), nil
}

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

// Full refs keep Git's short-name lookup from selecting a shadowing tag or local
// branch.
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
