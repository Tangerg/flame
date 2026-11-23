package workspace

import (
	"context"
	"errors"
	"fmt"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
)

type VCS struct{}

func (VCS) Changes(ctx context.Context, root string, maxChanges int) ([]workspaceapp.FileChange, error) {
	changes, err := git.ListChanges(ctx, root, maxChanges)
	if err != nil {
		return nil, vcsError(err)
	}
	out := make([]workspaceapp.FileChange, 0, len(changes))
	for _, change := range changes {
		status, ok := fileStatus(change.Status)
		if !ok {
			return nil, fmt.Errorf("workspace: unsupported git status %q", change.Status)
		}
		out = append(out, workspaceapp.FileChange{
			Path: change.Path, Status: status, PreviousPath: change.PreviousPath,
			Binary: change.Binary, Added: change.Added, Removed: change.Removed,
		})
	}
	return out, nil
}

func (VCS) StructuredDiff(
	ctx context.Context,
	root, path string,
	base bool,
	format workspaceapp.DiffFormat,
	budget workspaceapp.DiffBudget,
) (workspaceapp.StructuredDiffResult, error) {
	projection, err := diffProjection(format)
	if err != nil {
		return workspaceapp.StructuredDiffResult{}, err
	}
	result, err := git.Diff(ctx, root, path, diffMode(base), projection, git.DiffBudget{Files: budget.Files, Rows: budget.Rows, Bytes: budget.Bytes})
	if err != nil {
		return workspaceapp.StructuredDiffResult{}, vcsError(err)
	}
	baseline, err := diffBaseline(result.Baseline)
	if err != nil {
		return workspaceapp.StructuredDiffResult{}, err
	}
	out := make([]workspaceapp.FileDiff, 0, len(result.Files))
	for _, file := range result.Files {
		status, ok := fileStatus(file.Status)
		if !ok {
			return workspaceapp.StructuredDiffResult{}, fmt.Errorf("workspace: unsupported git status %q", file.Status)
		}
		rows := make([]workspaceapp.DiffRow, 0, len(file.Rows))
		for _, row := range file.Rows {
			kind, ok := diffRowType(row.Type)
			if !ok {
				return workspaceapp.StructuredDiffResult{}, fmt.Errorf("workspace: unsupported diff row type %q", row.Type)
			}
			rows = append(rows, workspaceapp.DiffRow{
				Type: kind, Text: row.Text, LeftLine: row.LeftLine, RightLine: row.RightLine, Code: row.Code, NoNewline: row.NoNewline,
			})
		}
		out = append(out, workspaceapp.FileDiff{
			Path: file.Path, Status: status, PreviousPath: file.PreviousPath,
			Binary: file.Binary, Added: file.Added, Removed: file.Removed, Rows: rows,
		})
	}
	return workspaceapp.StructuredDiffResult{
		Baseline: baseline, Files: out, Patch: result.Patch, Truncated: result.Truncated,
	}, nil
}

func (VCS) RawDiff(ctx context.Context, root, path string, base bool, maxBytes int) (workspaceapp.RawDiffResult, error) {
	result, err := git.RawDiff(ctx, root, path, diffMode(base), maxBytes)
	if err != nil {
		return workspaceapp.RawDiffResult{}, vcsError(err)
	}
	baseline, err := diffBaseline(result.Baseline)
	if err != nil {
		return workspaceapp.RawDiffResult{}, err
	}
	return workspaceapp.RawDiffResult{Baseline: baseline, Patch: result.Patch}, nil
}

func diffProjection(format workspaceapp.DiffFormat) (git.Projection, error) {
	switch format {
	case workspaceapp.DiffFormatRows:
		return git.Rows, nil
	case workspaceapp.DiffFormatVisual:
		return git.Visual, nil
	default:
		return "", fmt.Errorf("workspace: diff format %q has no structured projection", format)
	}
}

func diffBaseline(baseline git.Baseline) (workspaceapp.DiffBaseline, error) {
	var kind workspaceapp.DiffBaselineType
	switch baseline.Type {
	case git.BaselineHead:
		kind = workspaceapp.DiffBaselineHead
	case git.BaselineMergeBase:
		kind = workspaceapp.DiffBaselineMergeBase
	case git.BaselineEmptyTree:
		kind = workspaceapp.DiffBaselineEmptyTree
	default:
		return workspaceapp.DiffBaseline{}, fmt.Errorf("workspace: unsupported git baseline %q", baseline.Type)
	}
	return workspaceapp.DiffBaseline{Type: kind, Commit: baseline.Commit}, nil
}

func fileStatus(status git.Status) (workspaceapp.FileStatus, bool) {
	switch status {
	case git.StatusAdded:
		return workspaceapp.FileStatusAdded, true
	case git.StatusModified:
		return workspaceapp.FileStatusModified, true
	case git.StatusDeleted:
		return workspaceapp.FileStatusDeleted, true
	case git.StatusRenamed:
		return workspaceapp.FileStatusRenamed, true
	case git.StatusUntracked:
		return workspaceapp.FileStatusUntracked, true
	default:
		return "", false
	}
}

func diffRowType(kind git.RowType) (workspaceapp.DiffRowType, bool) {
	switch kind {
	case git.RowHunk:
		return workspaceapp.DiffRowHunk, true
	case git.RowContext:
		return workspaceapp.DiffRowContext, true
	case git.RowAdded:
		return workspaceapp.DiffRowAdded, true
	case git.RowDeleted:
		return workspaceapp.DiffRowDeleted, true
	default:
		return "", false
	}
}

func vcsError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, git.ErrNotRepo), errors.Is(err, git.ErrUnavailable):
		return errors.Join(workspaceapp.ErrVCSUnavailable, err)
	case errors.Is(err, git.ErrNoBase):
		return errors.Join(workspaceapp.ErrVCSBaseUnknown, err)
	case errors.Is(err, git.ErrResultTooLarge):
		return errors.Join(workspaceapp.ErrVCSResultTooLarge, err)
	default:
		return err
	}
}

func diffMode(base bool) git.Mode {
	if base {
		return git.Base
	}
	return git.Worktree
}
