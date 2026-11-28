package delivery

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

var diffFormats = wireVocabulary[workspaceapp.DiffFormat, protocol.DiffFormat]{
	{workspaceapp.DiffFormatRows, protocol.DiffFormatRows},
	{workspaceapp.DiffFormatRaw, protocol.DiffFormatRaw},
	{workspaceapp.DiffFormatVisual, protocol.DiffFormatVisual},
}

var diffModes = wireVocabulary[workspaceapp.DiffMode, protocol.DiffMode]{
	{workspaceapp.DiffModeWorktree, protocol.DiffModeWorktree},
	{workspaceapp.DiffModeBase, protocol.DiffModeBase},
}

var diffBaselines = wireVocabulary[workspaceapp.DiffBaselineType, protocol.DiffBaselineType]{
	{workspaceapp.DiffBaselineHead, protocol.DiffBaselineHead},
	{workspaceapp.DiffBaselineMergeBase, protocol.DiffBaselineMergeBase},
	{workspaceapp.DiffBaselineEmptyTree, protocol.DiffBaselineEmptyTree},
}

func (s *Handler) ListWorkspaceFileChanges(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	changes, err := s.workspaceVCS.Changes(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.WorkspaceFileChange, 0, len(changes))
	for _, change := range changes {
		entry, err := presentFileChange(change)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return protocol.NewPage(out), nil
}

func (s *Handler) GetWorkspaceDiff(ctx context.Context, in protocol.GetDiffRequest) (*protocol.Diff, error) {
	input, err := workspaceDiffInput(in)
	if err != nil {
		return nil, err
	}
	diff, err := s.workspaceVCS.Diff(ctx, input)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	return presentDiff(diff)
}

func workspaceDiffInput(in protocol.GetDiffRequest) (workspaceapp.DiffInput, error) {
	format, known := diffFormats.domain(in.Format)
	if !known && in.Format != "" {
		return workspaceapp.DiffInput{}, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown diff format %q", in.Format))
	}
	mode, known := diffModes.domain(in.Mode)
	switch {
	case in.Mode == "":
		mode = workspaceapp.DiffModeWorktree
	case !known:
		return workspaceapp.DiffInput{}, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown mode %q", in.Mode))
	}
	if in.Format == protocol.DiffFormatRaw && in.Limit != nil {
		return workspaceapp.DiffInput{}, protocol.NewFailure(protocol.ErrInvalidParams, "limit is only valid for rows or visual format")
	}
	rowLimit := workspaceapp.DefaultDiffRowLimit()
	if in.Limit != nil {
		explicit, err := workspaceapp.NewDiffRowLimit(*in.Limit)
		if err != nil {
			return workspaceapp.DiffInput{}, wireWorkspaceError(err)
		}
		rowLimit = explicit
	}
	return workspaceapp.DiffInput{
		CWD: in.Workspace.Path, Path: in.Path, Mode: mode, Format: format, RowLimit: rowLimit,
	}, nil
}

func presentDiff(diff workspaceapp.Diff) (*protocol.Diff, error) {
	baseline := protocol.DiffBaseline{Type: diffBaselines.mustWire(diff.Baseline.Type), Commit: diff.Baseline.Commit}
	if diff.Format == workspaceapp.DiffFormatRaw {
		return &protocol.Diff{Type: protocol.DiffFormatRaw, Baseline: baseline, Patch: &diff.Patch}, nil
	}
	budget := protocol.DiffBudget{Files: diff.Budget.Files, Rows: diff.Budget.Rows, Bytes: diff.Budget.Bytes}
	if diff.Format == workspaceapp.DiffFormatVisual {
		manifest, err := presentDiffManifest(diff.Files)
		if err != nil {
			return nil, err
		}
		return &protocol.Diff{Type: protocol.DiffFormatVisual, Baseline: baseline, Visual: &protocol.VisualDiff{
			Patch: diff.Patch, Manifest: manifest, Truncated: diff.Truncated, Budget: budget,
		}}, nil
	}
	files, err := presentDiffFiles(diff.Files)
	if err != nil {
		return nil, err
	}
	return &protocol.Diff{Type: protocol.DiffFormatRows, Baseline: baseline, Rows: &protocol.DiffRows{
		Files: files, Truncated: diff.Truncated, Budget: budget,
	}}, nil
}

func presentDiffManifest(files []workspaceapp.FileDiff) ([]protocol.WorkspaceFileChange, error) {
	manifest := make([]protocol.WorkspaceFileChange, 0, len(files))
	for _, file := range files {
		entry, err := presentFileChange(file.Change)
		if err != nil {
			return nil, err
		}
		manifest = append(manifest, entry)
	}
	return manifest, nil
}

func presentDiffFiles(files []workspaceapp.FileDiff) ([]protocol.FileDiff, error) {
	out := make([]protocol.FileDiff, 0, len(files))
	for _, file := range files {
		change, err := presentFileChange(file.Change)
		if err != nil {
			return nil, err
		}
		rows, err := presentDiffRows(file.Rows)
		if err != nil {
			return nil, err
		}
		out = append(out, protocol.FileDiff{
			Path: change.Path, Status: change.Status, PreviousPath: change.PreviousPath,
			Binary: change.Binary, Added: change.Added, Removed: change.Removed, Rows: rows,
		})
	}
	return out, nil
}

func presentDiffRows(rows []workspaceapp.DiffRow) ([]protocol.DiffRow, error) {
	out := make([]protocol.DiffRow, 0, len(rows))
	for _, row := range rows {
		kind, ok := presentDiffRowType(row.Type)
		if !ok {
			return nil, fmt.Errorf("workspace.diff.get: unsupported row type %q", row.Type)
		}
		entry := protocol.DiffRow{
			Type: kind, Text: row.Text, LeftLine: row.LeftLine, RightLine: row.RightLine, NoNewline: row.NoNewline,
		}
		if kind != protocol.DiffRowHunk {
			entry.Code = new(row.Code)
		}
		out = append(out, entry)
	}
	return out, nil
}

func presentFileChange(change workspaceapp.FileChange) (protocol.WorkspaceFileChange, error) {
	entry := protocol.WorkspaceFileChange{Path: change.Path, PreviousPath: change.PreviousPath, Binary: change.Binary}
	switch change.Status {
	case workspaceapp.FileStatusAdded:
		entry.Status = protocol.FileStatusAdded
	case workspaceapp.FileStatusModified:
		entry.Status = protocol.FileStatusModified
	case workspaceapp.FileStatusDeleted:
		entry.Status = protocol.FileStatusDeleted
	case workspaceapp.FileStatusRenamed:
		entry.Status = protocol.FileStatusRenamed
	case workspaceapp.FileStatusUntracked:
		entry.Status = protocol.FileStatusUntracked
	default:
		return protocol.WorkspaceFileChange{}, fmt.Errorf("workspace: unsupported file status %q", change.Status)
	}
	if !change.Binary {
		entry.Added, entry.Removed = new(change.Added), new(change.Removed)
	}
	return entry, nil
}

func presentDiffRowType(kind workspaceapp.DiffRowType) (protocol.DiffRowType, bool) {
	switch kind {
	case workspaceapp.DiffRowHunk:
		return protocol.DiffRowHunk, true
	case workspaceapp.DiffRowContext:
		return protocol.DiffRowContext, true
	case workspaceapp.DiffRowAdded:
		return protocol.DiffRowAdded, true
	case workspaceapp.DiffRowDeleted:
		return protocol.DiffRowDeleted, true
	default:
		return "", false
	}
}
