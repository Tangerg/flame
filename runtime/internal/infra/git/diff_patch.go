package git

import (
	"context"
	"fmt"
	"os"
)

type patchComparison struct {
	revision         string
	path             string
	includeUntracked bool
	binary           bool
}

func WorktreePatch(ctx context.Context, dir, revision string, maxBytes int) ([]byte, error) {
	patch, _, err := readWorktreePatch(ctx, dir, patchComparison{
		revision: revision, includeUntracked: true, binary: true,
	}, maxBytes)
	return patch, err
}

func readWorktreePatch(ctx context.Context, dir string, comparison patchComparison, maxBytes int) ([]byte, []string, error) {
	if maxBytes <= 0 {
		return nil, nil, fmt.Errorf("%w: diff requires a positive byte limit", ErrResultTooLarge)
	}
	pathspec, err := workspacePathspec(dir, comparison.path)
	if err != nil {
		return nil, nil, err
	}
	options := []string{"--no-ext-diff", "--no-textconv", "--no-color"}
	if comparison.binary {
		options = append(options, "--binary")
	}
	args := append([]string{"diff"}, options...)
	tracked, err := run(ctx, dir, append(args, "-M", "--relative", comparison.revision, "--", pathspec)...)
	if err != nil {
		return nil, nil, err
	}
	patch, err := appendDiffPatch(nil, tracked, maxBytes)
	if err != nil || !comparison.includeUntracked {
		return patch, nil, err
	}
	untracked, err := untrackedPaths(ctx, dir, pathspec)
	if err != nil {
		return nil, nil, err
	}
	noIndex := append([]string{"diff", "--no-index"}, options...)
	for _, path := range untracked {
		// Git uses status 1 when the file differs from /dev/null.
		delta, err := runAllowingExitCode(ctx, dir, 1, append(noIndex, "--", os.DevNull, path)...)
		if err != nil {
			return nil, nil, err
		}
		patch, err = appendDiffPatch(patch, delta, maxBytes)
		if err != nil {
			return nil, nil, err
		}
	}
	return patch, untracked, nil
}

func appendDiffPatch(patch, part []byte, maxBytes int) ([]byte, error) {
	if len(part) > maxBytes-len(patch) {
		return nil, fmt.Errorf("%w: aggregate diff exceeds %d bytes", ErrResultTooLarge, maxBytes)
	}
	if len(patch) == 0 {
		return part, nil
	}
	return append(patch, part...), nil
}
