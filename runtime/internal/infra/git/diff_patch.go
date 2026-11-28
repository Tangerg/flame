package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

const noIndexChanges = 1

var ErrUnpatchablePath = errors.New("git: path cannot be represented in a patch")

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
		delta, err := readUntrackedPatch(ctx, dir, append(noIndex, "--", os.DevNull, path))
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

func readUntrackedPatch(ctx context.Context, dir string, args []string) ([]byte, error) {
	result, err := process.At(ctx, dir, args...)
	if err != nil {
		return nil, gitProcessError(args, err)
	}
	// Git can also use status 1 when it cannot read the selected file.
	if result.ExitCode == noIndexChanges && len(result.Stdout) != 0 {
		return result.Stdout, nil
	}
	return nil, errors.Join(ErrUnpatchablePath, gitExitError(args, result.ExitCode, strings.TrimSpace(result.Stderr)))
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
