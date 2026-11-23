package runtimebinding

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type workspaceBinding interface {
	ResolveWorkspace(context.Context, protocol.ResolveWorkspaceRequest, protocol.CallOptions) (*protocol.WorkspaceInfo, error)
	ListWorkspaces(context.Context, protocol.CallOptions) (*protocol.Page[protocol.WorkspaceSummary], error)
	ListWorkspaceFileChanges(context.Context, protocol.WorkspaceQuery, protocol.CallOptions) (*protocol.Page[protocol.WorkspaceFileChange], error)
	GetWorkspaceDiff(context.Context, protocol.GetDiffRequest, protocol.CallOptions) (*protocol.Diff, error)
	SearchWorkspaceFiles(context.Context, protocol.GrepRequest, protocol.CallOptions) (*protocol.GrepResult, error)
	ListWorkspaceFiles(context.Context, protocol.ListFilesRequest, protocol.CallOptions) (*protocol.FileListPage, error)
	ReadWorkspaceFile(context.Context, protocol.ReadFileRequest, protocol.CallOptions) (*protocol.FileContent, error)
}

const (
	workspaceFilePageLimit           = 500
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
	return projectWorkspace(*resolved), nil
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
	summaries, err := projectUniqueValues(
		"list workspaces",
		values,
		projectWorkspaceSummary,
		func(summary workspace.Summary) string { return summary.Workspace.Path },
	)
	if err != nil {
		return nil, err
	}
	for _, summary := range summaries {
		if summary.LastActive == nil || summary.LastActive.IsZero() {
			return nil, runtimeContractViolation(
				"list workspaces returned workspace %q without an activity time", summary.Workspace.Path,
			)
		}
	}
	return summaries, nil
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
	changes, err := projectUniqueValues(
		"list workspace changes",
		values,
		projectChange,
		func(change workspace.Change) string { return change.Path },
	)
	if err != nil {
		return nil, err
	}
	return changes, nil
}

func (r *Connection) Diff(ctx context.Context, request workspace.DiffRequest) (workspace.Diff, error) {
	if err := request.Validate(); err != nil {
		return workspace.Diff{}, err
	}
	if err := r.requireFeature(protocol.FeatureGit); err != nil {
		return workspace.Diff{}, err
	}
	value, err := r.workspaces.GetWorkspaceDiff(ctx, protocol.GetDiffRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path,
		Mode: request.Mode, Format: request.Format, Limit: protocolOptionalPositiveInt(request.RowLimit.Rows()),
	}, r.callOptions())
	if err != nil {
		return workspace.Diff{}, classifyError(err)
	}
	if value == nil {
		return workspace.Diff{}, runtimeContractViolation("get workspace diff returned nil")
	}
	expected := request.Format
	if expected == "" {
		expected = protocol.DiffFormatRows
	}
	if value.Type != expected {
		return workspace.Diff{}, runtimeContractViolation("get workspace diff returned %q for %q", value.Type, expected)
	}
	projected, err := projectDiff(*value)
	if err != nil {
		return workspace.Diff{}, runtimeContractViolation("get workspace diff returned an invalid projection: %v", err)
	}
	return projected, nil
}

func (r *Connection) Search(ctx context.Context, request workspace.SearchRequest) (workspace.SearchResult, error) {
	if err := request.Validate(); err != nil {
		return workspace.SearchResult{}, err
	}
	value, err := r.workspaces.SearchWorkspaceFiles(ctx, protocol.GrepRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Query: request.Query,
		Path: request.Path, Limit: protocolOptionalPositiveInt(request.Limit.Matches()), ScanAll: request.ScanAll,
	}, r.callOptions())
	if err != nil {
		return workspace.SearchResult{}, classifyError(err)
	}
	if value == nil {
		return workspace.SearchResult{}, runtimeContractViolation("search workspace files returned nil")
	}
	result := workspace.SearchResult{Total: value.Total, Matches: value.Matches, Complete: value.Complete, Coverage: value.Coverage}
	if err := result.Validate(); err != nil {
		return workspace.SearchResult{}, runtimeContractViolation("search workspace files returned an invalid projection: %v", err)
	}
	return result, nil
}

func validateFilePage(page *protocol.FileListPage, cursor string) error {
	if page == nil {
		return runtimeContractViolation("list workspace files after cursor %q returned a nil page", cursor)
	}
	coverage := page.Coverage
	if page.Complete != (page.NextCursor == "") || coverage.ScannedEntries < len(page.Data) || coverage.ExcludedEntries < 0 || coverage.ExcludedEntries > coverage.ScannedEntries || (page.Complete && coverage.BudgetExhausted) {
		return runtimeContractViolation("list workspace files returned inconsistent completion or coverage")
	}
	if len(page.Data) > workspaceFilePageLimit {
		return runtimeContractViolation("list workspace files returned %d rows for limit %d", len(page.Data), workspaceFilePageLimit)
	}
	return nil
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
		cursor := cursors.cursor()
		page, err := r.workspaces.ListWorkspaceFiles(ctx, protocol.ListFilesRequest{
			Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path, Glob: request.Glob,
			Recursive: request.Recursive, IncludeIgnored: request.IncludeIgnored,
			PageQuery: protocol.PageQuery{Limit: protocolPositiveInt(workspaceFilePageLimit), Cursor: cursor},
		}, r.callOptions())
		if err != nil {
			return workspace.FileListing{}, classifyError(err)
		}
		if err := validateFilePage(page, cursor); err != nil {
			return workspace.FileListing{}, err
		}
		for _, entry := range page.Data {
			result.Entries = append(result.Entries, workspace.FileEntry{
				Path: entry.Path, Type: entry.Type, SizeBytes: entry.SizeBytes, ModifiedAt: entry.ModifiedAt,
			})
		}
		more, err := cursors.advance(page.NextCursor)
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
	start, end := request.Range.Bounds()
	value, err := r.workspaces.ReadWorkspaceFile(ctx, protocol.ReadFileRequest{
		Workspace: protocol.WorkspaceRef{Path: request.Workspace}, Path: request.Path,
		StartLine: protocolOptionalPositiveInt(start), EndLine: protocolOptionalPositiveInt(end),
		MaxBytes: protocolOptionalPositiveInt(request.ByteLimit.Bytes()),
	}, r.callOptions())
	if err != nil {
		return workspace.FileContent{}, classifyError(err)
	}
	if value == nil {
		return workspace.FileContent{}, runtimeContractViolation("read workspace file returned nil")
	}
	result := workspace.FileContent{
		Content: value.Content, TotalLines: value.TotalLines,
		Truncated: value.Truncated, StartLine: value.StartLine, EndLine: value.EndLine,
	}
	if err := result.Validate(); err != nil {
		return workspace.FileContent{}, runtimeContractViolation("read workspace file returned an unreadable window: %v", err)
	}
	return result, nil
}

func projectWorkspace(value protocol.WorkspaceInfo) workspace.Workspace {
	return workspace.Workspace{
		Path: value.Ref.Path, ProjectRoot: value.ProjectRoot, Availability: value.Availability,
	}
}

func projectWorkspaceSummary(value protocol.WorkspaceSummary) workspace.Summary {
	return workspace.Summary{
		Workspace:  projectWorkspace(value.Workspace),
		Name:       value.Name,
		Sessions:   value.SessionCount,
		LastActive: value.LastActiveAt,
	}
}

func projectChange(value protocol.WorkspaceFileChange) workspace.Change {
	return workspace.Change{
		Path: value.Path, Status: value.Status, PreviousPath: value.PreviousPath,
		Added: value.Added, Removed: value.Removed, Binary: value.Binary,
	}
}

func projectDiff(value protocol.Diff) (workspace.Diff, error) {
	if err := protocol.ValidateWireTree(value); err != nil {
		return workspace.Diff{}, fmt.Errorf("get workspace diff projection: %w", err)
	}
	result := workspace.Diff{Baseline: value.Baseline}
	if value.Type == protocol.DiffFormatRaw {
		result.Patch = *value.Patch
		return result, nil
	}
	if value.Type != protocol.DiffFormatRows {
		return workspace.Diff{}, fmt.Errorf("get workspace diff projection: unsupported format %q", value.Type)
	}
	result.Truncated = value.Rows.Truncated
	result.Files = make([]workspace.FileDiff, 0, len(value.Rows.Files))
	for _, file := range value.Rows.Files {
		result.Files = append(result.Files, workspace.FileDiff{
			Change: projectChange(protocol.WorkspaceFileChange{
				Path: file.Path, Status: file.Status, PreviousPath: file.PreviousPath,
				Added: file.Added, Removed: file.Removed, Binary: file.Binary,
			}),
			Rows: file.Rows,
		})
	}
	if err := result.Validate(); err != nil {
		return workspace.Diff{}, fmt.Errorf("get workspace diff projection: %w", err)
	}
	return result, nil
}
