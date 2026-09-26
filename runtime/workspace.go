package runtime

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ResolveWorkspace resolves and validates a workspace reference.
func (r *binding) ResolveWorkspace(ctx context.Context, request protocol.ResolveWorkspaceRequest, options CallOptions) (*protocol.WorkspaceInfo, error) {
	return r.invoke[protocol.ResolveWorkspaceRequest, *protocol.WorkspaceInfo](ctx, delivery.WorkspacesResolve, request, callOptions(options))
}

// ListWorkspaces returns the Runtime's discoverable workspaces.
func (r *binding) ListWorkspaces(ctx context.Context, options CallOptions) (*protocol.Page[protocol.WorkspaceSummary], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.WorkspaceSummary]](ctx, delivery.WorkspacesList, struct{}{}, callOptions(options))
}

// ListWorkspaceFileChanges returns version-control changes in a workspace.
func (r *binding) ListWorkspaceFileChanges(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.WorkspaceFileChange]](ctx, delivery.WorkspaceChangesList, request, callOptions(options))
}

// GetWorkspaceDiff returns a workspace diff.
func (r *binding) GetWorkspaceDiff(ctx context.Context, request protocol.GetDiffRequest, options CallOptions) (*protocol.Diff, error) {
	return r.invoke[protocol.GetDiffRequest, *protocol.Diff](ctx, delivery.WorkspaceDiffGet, request, callOptions(options))
}

// GetWorkspaceFileHead returns metadata for one workspace file.
func (r *binding) GetWorkspaceFileHead(ctx context.Context, request protocol.GetFileHeadRequest, options CallOptions) (*protocol.FileHead, error) {
	return r.invoke[protocol.GetFileHeadRequest, *protocol.FileHead](ctx, delivery.WorkspaceFilesHead, request, callOptions(options))
}

// SearchWorkspaceFiles searches text within a workspace.
func (r *binding) SearchWorkspaceFiles(ctx context.Context, request protocol.GrepRequest, options CallOptions) (*protocol.GrepResult, error) {
	return r.invoke[protocol.GrepRequest, *protocol.GrepResult](ctx, delivery.WorkspaceFilesSearch, request, callOptions(options))
}

// ListWorkspaceFiles returns one cursor page of workspace entries.
func (r *binding) ListWorkspaceFiles(ctx context.Context, request protocol.ListFilesRequest, options CallOptions) (*protocol.Page[protocol.FileEntry], error) {
	return r.invoke[protocol.ListFilesRequest, *protocol.Page[protocol.FileEntry]](ctx, delivery.WorkspaceFilesList, request, callOptions(options))
}

// ReadWorkspaceFile reads one workspace file.
func (r *binding) ReadWorkspaceFile(ctx context.Context, request protocol.ReadFileRequest, options CallOptions) (*protocol.FileContent, error) {
	return r.invoke[protocol.ReadFileRequest, *protocol.FileContent](ctx, delivery.WorkspaceFilesRead, request, callOptions(options))
}
