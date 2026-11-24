package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (s *Handler) ResolveWorkspace(_ context.Context, in protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error) {
	path := ""
	if in.Ref != nil {
		path = in.Ref.Path
	}
	resolved, err := s.workspaceDiscovery.Resolve(path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := presentWorkspaceInfo(resolved.Path, resolved.RepositoryRoot, resolved.Missing)
	return &out, nil
}

func (s *Handler) ListWorkspaces(ctx context.Context) (*protocol.Page[protocol.WorkspaceSummary], error) {
	workspaces, err := s.workspaceDiscovery.Workspaces(ctx)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.WorkspaceSummary, 0, len(workspaces))
	for _, workspace := range workspaces {
		lastActiveAt := workspace.LastActiveAt
		out = append(out, protocol.WorkspaceSummary{
			Workspace: presentWorkspaceInfo(workspace.Path, workspace.RepositoryRoot, workspace.Missing),
			Name:      workspace.Name, SessionCount: workspace.SessionCount, LastActiveAt: &lastActiveAt,
		})
	}
	return protocol.NewPage(out), nil
}

func presentWorkspaceInfo(path, repositoryRoot string, missing bool) protocol.WorkspaceInfo {
	availability := protocol.WorkspaceAvailable
	if missing {
		availability = protocol.WorkspaceMissing
	}
	return protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: path}, RepositoryRoot: repositoryRoot, Availability: availability,
	}
}
