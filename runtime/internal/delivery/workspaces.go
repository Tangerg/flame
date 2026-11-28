package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
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
	out := presentWorkspaceInfo(resolved)
	return &out, nil
}

func (s *Handler) ListWorkspaces(ctx context.Context) (*protocol.Page[protocol.WorkspaceSummary], error) {
	workspaces, err := s.workspaceDiscovery.Workspaces(ctx)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.WorkspaceSummary, 0, len(workspaces))
	for _, summary := range workspaces {
		lastActiveAt := summary.LastActiveAt
		out = append(out, protocol.WorkspaceSummary{
			Workspace: presentWorkspaceInfo(summary.Workspace),
			Name:      summary.Name, SessionCount: summary.SessionCount, LastActiveAt: &lastActiveAt,
		})
	}
	return protocol.NewPage(out), nil
}

func presentWorkspaceInfo(workspace workspaceapp.Info) protocol.WorkspaceInfo {
	availability := protocol.WorkspaceAvailable
	if workspace.Missing {
		availability = protocol.WorkspaceMissing
	}
	return protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: workspace.Path}, RepositoryRoot: workspace.RepositoryRoot, Availability: availability,
	}
}
