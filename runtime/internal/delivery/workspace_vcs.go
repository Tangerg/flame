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
		status, ok := presentFileStatus(change.Status)
		if !ok {
			return nil, fmt.Errorf("workspace.changes.list: unsupported file status %q", change.Status)
		}
		entry := protocol.WorkspaceFileChange{
			Path: change.Path, Status: status, PreviousPath: change.PreviousPath, Binary: change.Binary,
		}
		if !change.Binary {
			added, removed := change.Added, change.Removed
			entry.Added, entry.Removed = &added, &removed
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
	var base bool
	switch in.Mode {
	case "", protocol.DiffModeWorktree:
	case protocol.DiffModeBase:
		base = true
	default:
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
		CWD: in.Workspace.Path, Path: in.Path, Base: base, Format: format, RowLimit: rowLimit,
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
		status, ok := presentFileStatus(file.Status)
		if !ok {
			return nil, fmt.Errorf("workspace.diff.get: unsupported file status %q", file.Status)
		}
		entry := protocol.WorkspaceFileChange{Path: file.Path, Status: status, PreviousPath: file.PreviousPath, Binary: file.Binary}
		if !file.Binary {
			entry.Added, entry.Removed = new(file.Added), new(file.Removed)
		}
		manifest = append(manifest, entry)
	}
	return manifest, nil
}

func presentDiffFiles(files []workspaceapp.FileDiff) ([]protocol.FileDiff, error) {
	out := make([]protocol.FileDiff, 0, len(files))
	for _, file := range files {
		status, ok := presentFileStatus(file.Status)
		if !ok {
			return nil, fmt.Errorf("workspace.diff.get: unsupported file status %q", file.Status)
		}
		rows, err := presentDiffRows(file.Rows)
		if err != nil {
			return nil, err
		}
		entry := protocol.FileDiff{
			Path: file.Path, Status: status, PreviousPath: file.PreviousPath,
			Binary: file.Binary, Rows: rows,
		}
		if !file.Binary {
			added, removed := file.Added, file.Removed
			entry.Added, entry.Removed = &added, &removed
		}
		out = append(out, entry)
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

func presentFileStatus(status workspaceapp.FileStatus) (protocol.FileStatus, bool) {
	switch status {
	case workspaceapp.FileStatusAdded:
		return protocol.FileStatusAdded, true
	case workspaceapp.FileStatusModified:
		return protocol.FileStatusModified, true
	case workspaceapp.FileStatusDeleted:
		return protocol.FileStatusDeleted, true
	case workspaceapp.FileStatusRenamed:
		return protocol.FileStatusRenamed, true
	case workspaceapp.FileStatusUntracked:
		return protocol.FileStatusUntracked, true
	default:
		return "", false
	}
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
