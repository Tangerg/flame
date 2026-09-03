package runtimebinding

import (
	"context"
	"fmt"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type workspaceBinding interface {
	ResolveWorkspace(context.Context, protocol.ResolveWorkspaceRequest, flameruntime.CallOptions) (*protocol.WorkspaceInfo, error)
	ListWorkspaces(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.WorkspaceSummary], error)
	ListWorkspaceFileChanges(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.WorkspaceFileChange], error)
	GetWorkspaceDiff(context.Context, protocol.GetDiffRequest, flameruntime.CallOptions) (*protocol.Diff, error)
	GetWorkspaceFileHead(context.Context, protocol.GetFileHeadRequest, flameruntime.CallOptions) (*protocol.FileHead, error)
	SearchWorkspaceFiles(context.Context, protocol.GrepRequest, flameruntime.CallOptions) (*protocol.GrepResult, error)
	ListWorkspaceFiles(context.Context, protocol.ListFilesRequest, flameruntime.CallOptions) (*protocol.Page[protocol.FileEntry], error)
	ReadWorkspaceFile(context.Context, protocol.ReadFileRequest, flameruntime.CallOptions) (*protocol.FileContent, error)
}

const (
	workspaceFilePageLimit = 500
	// maximumWorkspaceFilePageRequests matches Flame's 20,000-entry
	// authoritative workspace listing boundary at 500 rows per request.
	maximumWorkspaceFilePageRequests = 40
)

func (r *Connection) Resolve(ctx context.Context, request workspace.ResolveRequest) (workspace.Workspace, error) {
	if err := request.Validate(); err != nil {
		return workspace.Workspace{}, err
	}
	wire := protocol.ResolveWorkspaceRequest{}
	if request.Path != "" {
		wire.Ref = &protocol.WorkspaceRef{Path: request.Path}
	}
	resolved, err := r.workspaces.ResolveWorkspace(ctx, wire, r.callOptions())
	if err != nil {
		return workspace.Workspace{}, classifyError(err)
	}
	if resolved == nil {
		return workspace.Workspace{}, runtimeContractViolation("resolve workspace returned nil")
	}
	projected, err := projectWorkspace(*resolved)
	if err != nil {
		return workspace.Workspace{}, runtimeContractViolation("resolve workspace returned an invalid workspace: %v", err)
	}
	return projected, nil
}

func (r *Connection) List(ctx context.Context) ([]workspace.Summary, error) {
	page, err := r.workspaces.ListWorkspaces(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list workspaces", page)
	if err != nil {
		return nil, err
	}
	return projectUniqueValuesFallible(
		"list workspaces",
		values,
		projectWorkspaceSummary,
		func(summary workspace.Summary) string { return summary.Workspace.Path },
	)
}

func (r *Connection) Changes(ctx context.Context, path string) ([]workspace.Change, error) {
	if err := r.requireFeature(protocol.FeatureGit); err != nil {
		return nil, err
	}
	page, err := r.workspaces.ListWorkspaceFileChanges(ctx, protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: path},
	}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list workspace changes", page)
	if err != nil {
		return nil, err
	}
	return projectUniqueValuesFallible(
		"list workspace changes",
		values,
		func(value protocol.WorkspaceFileChange) (workspace.Change, error) {
			return projectChange(value.Path, value.Status, value.PreviousPath, value.Added, value.Removed, value.Binary)
		},
		func(change workspace.Change) string { return change.Path },
	)
}

func (r *Connection) Diff(ctx context.Context, request workspace.DiffRequest) (workspace.Diff, error) {
	if err := request.Validate(); err != nil {
		return workspace.Diff{}, err
	}
	if err := r.requireFeature(protocol.FeatureGit); err != nil {
		return workspace.Diff{}, err
	}
	var rowLimit *int
	rows, explicit, err := request.RowLimit.Rows()
	if err != nil {
		return workspace.Diff{}, err
	}
	if explicit {
		rowLimit = protocolPositiveInt(rows)
	}
	value, err := r.workspaces.GetWorkspaceDiff(ctx, protocol.GetDiffRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path,
		Mode: request.Mode, Format: request.Format, Limit: rowLimit,
	}, r.callOptions())
	if err != nil {
		return workspace.Diff{}, classifyError(err)
	}
	if value == nil {
		return workspace.Diff{}, runtimeContractViolation("get workspace diff returned nil")
	}
	projected, err := projectDiff(*value)
	if err != nil {
		return workspace.Diff{}, runtimeContractViolation("get workspace diff returned an invalid projection: %v", err)
	}
	return projected, nil
}

func (r *Connection) Head(ctx context.Context, request workspace.HeadRequest) (workspace.FileHead, error) {
	if err := request.Validate(); err != nil {
		return workspace.FileHead{}, err
	}
	lines, err := request.LineLimit.Lines()
	if err != nil {
		return workspace.FileHead{}, err
	}
	value, err := r.workspaces.GetWorkspaceFileHead(ctx, protocol.GetFileHeadRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path, Lines: protocolPositiveInt(lines),
	}, r.callOptions())
	if err != nil {
		return workspace.FileHead{}, classifyError(err)
	}
	if value == nil {
		return workspace.FileHead{}, runtimeContractViolation("get workspace file head returned nil")
	}
	result := workspace.FileHead{Path: value.Path, Lines: make([]workspace.FileLine, 0, len(value.Lines))}
	for _, line := range value.Lines {
		result.Lines = append(result.Lines, workspace.FileLine{Number: line.LineNumber, Text: line.Text})
	}
	if err := result.Validate(); err != nil {
		return workspace.FileHead{}, runtimeContractViolation("get workspace file head returned an invalid projection: %v", err)
	}
	return result, nil
}

func (r *Connection) Search(ctx context.Context, request workspace.SearchRequest) (workspace.SearchResult, error) {
	if err := request.Validate(); err != nil {
		return workspace.SearchResult{}, err
	}
	limit, err := request.Limit.Matches()
	if err != nil {
		return workspace.SearchResult{}, err
	}
	value, err := r.workspaces.SearchWorkspaceFiles(ctx, protocol.GrepRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Query: request.Query,
		Path: request.Path, Limit: protocolPositiveInt(limit),
	}, r.callOptions())
	if err != nil {
		return workspace.SearchResult{}, classifyError(err)
	}
	if value == nil {
		return workspace.SearchResult{}, runtimeContractViolation("search workspace files returned nil")
	}
	result := workspace.SearchResult{Total: value.Total, Matches: make([]workspace.Match, 0, len(value.Matches))}
	for _, match := range value.Matches {
		result.Matches = append(result.Matches, workspace.Match{Path: match.Path, Line: match.LineNumber, Text: match.Text})
	}
	if err := result.Validate(); err != nil {
		return workspace.SearchResult{}, runtimeContractViolation("search workspace files returned an invalid projection: %v", err)
	}
	return result, nil
}

func (r *Connection) Files(ctx context.Context, request workspace.FilesRequest) (workspace.FileListing, error) {
	if err := request.Validate(); err != nil {
		return workspace.FileListing{}, err
	}
	result := workspace.FileListing{}
	cursors, err := newCursorTraversal("list workspace files", "", maximumWorkspaceFilePageRequests)
	if err != nil {
		return workspace.FileListing{}, err
	}
	for {
		if err := context.Cause(ctx); err != nil {
			return workspace.FileListing{}, err
		}
		cursor := cursors.Current()
		page, err := r.workspaces.ListWorkspaceFiles(ctx, protocol.ListFilesRequest{
			Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path, Glob: request.Glob,
			Recursive: request.Recursive, IncludeIgnored: request.IncludeIgnored,
			PageQuery: protocol.PageQuery{Limit: protocolPositiveInt(workspaceFilePageLimit), Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return workspace.FileListing{}, classifyError(err)
		}
		if page == nil {
			return workspace.FileListing{}, runtimeContractViolation("list workspace files after cursor %q returned a nil page", cursor)
		}
		if len(page.Data) > workspaceFilePageLimit {
			return workspace.FileListing{}, runtimeContractViolation(
				"list workspace files returned %d rows for limit %d",
				len(page.Data),
				workspaceFilePageLimit,
			)
		}
		for _, entry := range page.Data {
			result.Entries = append(result.Entries, workspace.FileEntry{
				Path: entry.Path, Name: entry.Name, Type: entry.Type,
				SizeBytes: cloneInt64(entry.SizeBytes), ModifiedAt: entry.ModifiedAt,
			})
		}
		more, err := cursors.Advance(page.NextCursor)
		if err != nil {
			return workspace.FileListing{}, err
		}
		if !more {
			break
		}
	}
	if err := result.Validate(); err != nil {
		return workspace.FileListing{}, runtimeContractViolation("list workspace files returned an invalid projection: %v", err)
	}
	return result, nil
}

func (r *Connection) Read(ctx context.Context, request workspace.ReadRequest) (workspace.FileContent, error) {
	if err := request.Validate(); err != nil {
		return workspace.FileContent{}, err
	}
	start, end, err := request.Range.Bounds()
	if err != nil {
		return workspace.FileContent{}, err
	}
	maxBytes, err := request.ByteLimit.Bytes()
	if err != nil {
		return workspace.FileContent{}, err
	}
	var startLine, endLine *int
	if start > 0 {
		startLine = protocolPositiveInt(start)
	}
	if end > 0 {
		endLine = protocolPositiveInt(end)
	}
	value, err := r.workspaces.ReadWorkspaceFile(ctx, protocol.ReadFileRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path,
		StartLine: startLine, EndLine: endLine, MaxBytes: protocolPositiveInt(maxBytes),
	}, r.callOptions())
	if err != nil {
		return workspace.FileContent{}, classifyError(err)
	}
	if value == nil {
		return workspace.FileContent{}, runtimeContractViolation("read workspace file returned nil")
	}
	result := workspace.FileContent{
		Path: value.Path, Content: value.Content, Encoding: value.Encoding, TotalLines: value.TotalLines,
		Truncated: value.Truncated, StartLine: value.StartLine, EndLine: value.EndLine,
	}
	if err := result.Validate(); err != nil {
		return workspace.FileContent{}, runtimeContractViolation("read workspace file returned an invalid projection: %v", err)
	}
	return result, nil
}

func projectWorkspace(value protocol.WorkspaceInfo) (workspace.Workspace, error) {
	if err := protocol.ValidateWireTree(value); err != nil {
		return workspace.Workspace{}, fmt.Errorf("runtime workspace %q: %w", value.Ref.Path, err)
	}
	result := workspace.Workspace{
		Path: value.Ref.Path, ProjectRoot: value.ProjectRoot, Availability: value.Availability,
	}
	if err := result.Validate(); err != nil {
		return workspace.Workspace{}, fmt.Errorf("runtime workspace %q: %w", value.Ref.Path, err)
	}
	return result, nil
}

func projectWorkspaceSummary(value protocol.WorkspaceSummary) (workspace.Summary, error) {
	projected, err := projectWorkspace(value.Workspace)
	if err != nil {
		return workspace.Summary{}, err
	}
	result := workspace.Summary{
		Workspace: projected, Name: value.Name, Sessions: value.SessionCount,
	}
	if value.LastActiveAt != nil {
		result.LastActive = new(*value.LastActiveAt)
	}
	if err := result.Validate(); err != nil {
		return workspace.Summary{}, err
	}
	return result, nil
}

func projectChange(path string, status protocol.FileStatus, previousPath string, added, removed *int, binary bool) (workspace.Change, error) {
	result := workspace.Change{
		Path: path, Status: status, PreviousPath: previousPath,
		Added: cloneInt(added), Removed: cloneInt(removed), Binary: binary,
	}
	if err := result.Validate(); err != nil {
		return workspace.Change{}, err
	}
	return result, nil
}

func projectDiff(value protocol.Diff) (workspace.Diff, error) {
	result := workspace.Diff{Patch: value.Patch, Truncated: value.Truncated, Files: make([]workspace.FileDiff, 0, len(value.Files))}
	for index, file := range value.Files {
		change, err := projectChange(file.Path, file.Status, file.PreviousPath, file.Added, file.Removed, file.Binary)
		if err != nil {
			return workspace.Diff{}, fmt.Errorf("workspace file diff %d: %w", index, err)
		}
		rows := make([]workspace.DiffRow, 0, len(file.Rows))
		for _, row := range file.Rows {
			rows = append(rows, workspace.DiffRow{
				Type: row.Type, Text: row.Text, LeftLine: row.LeftLine,
				RightLine: row.RightLine, Code: row.Code,
			})
		}
		result.Files = append(result.Files, workspace.FileDiff{Change: change, Rows: rows})
	}
	if err := result.Validate(); err != nil {
		return workspace.Diff{}, fmt.Errorf("get workspace diff projection: %w", err)
	}
	return result, nil
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	return new(*value)
}
