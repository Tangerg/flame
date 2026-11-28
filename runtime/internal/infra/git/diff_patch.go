package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

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
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color"}
	if comparison.binary {
		args = append(args, "--binary")
	}
	args = append(args, "-M", "--relative", comparison.revision, "--", pathspec)
	var untracked []string
	result, err := process.ObserveWorktree(ctx, dir, func(query process.WorktreeQuery) (process.Result, error) {
		if comparison.includeUntracked {
			paths, err := untrackedPaths(query, pathspec)
			if err != nil {
				return process.Result{}, err
			}
			if err := includeUntrackedPaths(query, paths); err != nil {
				return process.Result{}, err
			}
			untracked = paths
		}
		return query(nil, args...)
	})
	patch, err := readCommandOutput(args, result, err, 0)
	if err != nil {
		return nil, nil, err
	}
	if len(patch) > maxBytes {
		return nil, nil, fmt.Errorf("%w: diff exceeds %d bytes", ErrResultTooLarge, maxBytes)
	}
	return patch, untracked, nil
}

func includeUntrackedPaths(query process.WorktreeQuery, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	var names bytes.Buffer
	for _, path := range paths {
		names.WriteString(path)
		names.WriteByte(0)
	}
	args := []string{"add", "--intent-to-add", "--pathspec-from-file=-", "--pathspec-file-nul"}
	result, err := query(&names, append([]string{"--literal-pathspecs"}, args...)...)
	if err != nil {
		return gitProcessError(args, err)
	}
	if result.ExitCode != 0 {
		return errors.Join(ErrUnpatchablePath, gitExitError(args, result.ExitCode, strings.TrimSpace(result.Stderr)))
	}
	return nil
}
