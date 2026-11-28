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
		converted, err := fileChange(change)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func (VCS) StructuredDiff(
	ctx context.Context,
	root, path string,
	mode workspaceapp.DiffMode,
	format workspaceapp.DiffFormat,
	budget workspaceapp.DiffBudget,
) (workspaceapp.StructuredDiffResult, error) {
	projection, err := diffProjection(format)
	if err != nil {
		return workspaceapp.StructuredDiffResult{}, err
	}
	source, err := diffSource(mode)
	if err != nil {
		return workspaceapp.StructuredDiffResult{}, err
	}
	result, err := git.Diff(ctx, root, path, source, projection, git.DiffBudget{Files: budget.Files, Rows: budget.Rows, Bytes: budget.Bytes})
	if err != nil {
		return workspaceapp.StructuredDiffResult{}, vcsError(err)
	}
	baseline, err := diffBaseline(result.Baseline)
	if err != nil {
		return workspaceapp.StructuredDiffResult{}, err
	}
	out := make([]workspaceapp.FileDiff, 0, len(result.Files))
	for _, file := range result.Files {
		change, err := fileChange(file.FileChange)
		if err != nil {
			return workspaceapp.StructuredDiffResult{}, err
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
		out = append(out, workspaceapp.FileDiff{Change: change, Rows: rows})
	}
	return workspaceapp.StructuredDiffResult{
		Baseline: baseline, Files: out, Patch: result.Patch, Truncated: result.Truncated,
	}, nil
}

func (VCS) RawDiff(ctx context.Context, root, path string, mode workspaceapp.DiffMode, maxBytes int) (workspaceapp.RawDiffResult, error) {
	source, err := diffSource(mode)
	if err != nil {
		return workspaceapp.RawDiffResult{}, err
	}
	result, err := git.RawDiff(ctx, root, path, source, maxBytes)
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

func fileChange(change git.FileChange) (workspaceapp.FileChange, error) {
	out := workspaceapp.FileChange{
		Path: change.Path, PreviousPath: change.PreviousPath,
		Binary: change.Binary, Added: change.Added, Removed: change.Removed,
	}
	switch change.Status {
	case git.StatusAdded:
		out.Status = workspaceapp.FileStatusAdded
	case git.StatusModified:
		out.Status = workspaceapp.FileStatusModified
	case git.StatusDeleted:
		out.Status = workspaceapp.FileStatusDeleted
	case git.StatusRenamed:
		out.Status = workspaceapp.FileStatusRenamed
	case git.StatusUntracked:
		out.Status = workspaceapp.FileStatusUntracked
	default:
		return workspaceapp.FileChange{}, fmt.Errorf("workspace: unsupported git status %q", change.Status)
	}
	return out, nil
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
	case errors.Is(err, git.ErrUnpatchablePath):
		return errors.Join(workspaceapp.ErrVCSUnpatchablePath, err)
	default:
		return err
	}
}

func diffSource(mode workspaceapp.DiffMode) (git.Mode, error) {
	switch mode {
	case workspaceapp.DiffModeWorktree:
		return git.Worktree, nil
	case workspaceapp.DiffModeBase:
		return git.Base, nil
	default:
		return "", fmt.Errorf("workspace: unknown diff mode %q", mode)
	}
}
