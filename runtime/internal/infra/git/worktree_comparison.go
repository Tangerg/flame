package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/git/process"
)

var ErrUnpatchablePath = errors.New("git: path cannot be represented in a patch")

const maxUntrackedDiffPaths = 10_000

type worktreeComparison struct {
	revision         string
	path             string
	includeUntracked bool
}

type worktreeComparisonOutput struct {
	data      []byte
	untracked map[string]struct{}
}

func (o worktreeComparisonOutput) status(path string, status Status) Status {
	if _, untracked := o.untracked[path]; untracked && status == StatusAdded {
		return StatusUntracked
	}
	return status
}

func WorktreePatch(ctx context.Context, dir, revision string, maxBytes int) ([]byte, error) {
	comparison := worktreeComparison{revision: revision, includeUntracked: true}
	output, err := comparison.patch(ctx, dir, maxBytes, "--binary")
	return output.data, err
}

func (comparison worktreeComparison) patch(ctx context.Context, dir string, maxBytes int, flags ...string) (worktreeComparisonOutput, error) {
	if maxBytes <= 0 {
		return worktreeComparisonOutput{}, fmt.Errorf("%w: diff requires a positive byte limit", ErrResultTooLarge)
	}
	output, err := comparison.read(ctx, dir, flags...)
	if err != nil {
		return worktreeComparisonOutput{}, err
	}
	if len(output.data) > maxBytes {
		return worktreeComparisonOutput{}, fmt.Errorf("%w: diff exceeds %d bytes", ErrResultTooLarge, maxBytes)
	}
	return output, nil
}

func (comparison worktreeComparison) read(ctx context.Context, dir string, flags ...string) (worktreeComparisonOutput, error) {
	pathspec, err := workspacePathspec(dir, comparison.path)
	if err != nil {
		return worktreeComparisonOutput{}, err
	}
	args := append([]string{"diff", "--no-ext-diff", "--no-textconv", "--no-color"}, flags...)
	args = append(args, "-M", "--relative", comparison.revision, "--", pathspec)
	var untracked map[string]struct{}
	result, err := process.ObserveWorktree(ctx, dir, func(query process.WorktreeQuery) (process.Result, error) {
		if comparison.includeUntracked {
			paths, err := untrackedPaths(query, pathspec)
			if err != nil {
				return process.Result{}, err
			}
			if err := includeUntrackedPaths(query, paths); err != nil {
				return process.Result{}, err
			}
			untracked = make(map[string]struct{}, len(paths))
			for _, path := range paths {
				untracked[path] = struct{}{}
			}
		}
		return query(nil, args...)
	})
	data, err := readCommandOutput(args, result, err, 0)
	if err != nil {
		return worktreeComparisonOutput{}, err
	}
	return worktreeComparisonOutput{data: data, untracked: untracked}, nil
}

func untrackedPaths(query process.WorktreeQuery, pathspec string) ([]string, error) {
	args := []string{"ls-files", "--others", "--exclude-standard", "-z", "--", pathspec}
	result, err := query(nil, args...)
	out, err := readCommandOutput(args, result, err, 0)
	if err != nil {
		return nil, err
	}
	var paths []string
	for raw := range bytes.SplitSeq(out, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		path := string(raw)
		if !filepath.IsLocal(path) {
			return nil, fmt.Errorf("git: untracked path %q lies outside the working tree", path)
		}
		if strings.HasSuffix(path, "/") {
			return nil, fmt.Errorf("%w: untracked directory %q", ErrUnpatchablePath, path)
		}
		if len(paths) >= maxUntrackedDiffPaths {
			return nil, fmt.Errorf("%w: more than %d untracked diff paths", ErrResultTooLarge, maxUntrackedDiffPaths)
		}
		paths = append(paths, path)
	}
	return paths, nil
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
