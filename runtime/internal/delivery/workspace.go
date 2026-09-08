package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	WorkspacesResolve    Name = "workspaces.resolve"
	WorkspacesList       Name = "workspaces.list"
	WorkspaceChangesList Name = "workspace.changes.list"
	WorkspaceDiffGet     Name = "workspace.diff.get"
	WorkspaceFilesHead   Name = "workspace.files.head"
	WorkspaceFilesSearch Name = "workspace.files.search"
	WorkspaceFilesList   Name = "workspace.files.list"
	WorkspaceFilesRead   Name = "workspace.files.read"
)

func registerWorkspace(registry *Registry) {
	registry.Query(MethodMeta{
		Name:   WorkspacesResolve,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, (*Handler).ResolveWorkspace)

	registry.Query(MethodMeta{Name: WorkspacesList},
		func(service *Handler, ctx context.Context, _ struct{}) (*protocol.Page[protocol.WorkspaceSummary], error) {
			return service.ListWorkspaces(ctx)
		})

	// Git reads require the advertised capability. Once negotiated, a path that is
	// not a repository is the distinct vcs_unavailable domain answer.
	registry.Query(MethodMeta{
		Name: WorkspaceChangesList,
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
	}, (*Handler).ListWorkspaceFileChanges)

	registry.Query(MethodMeta{
		Name: WorkspaceDiffGet,
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
	}, (*Handler).GetWorkspaceDiff)

	registry.Query(MethodMeta{
		Name: WorkspaceFilesHead,
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
			protocol.ErrUnsupportedMime.Error(),
		},
	}, (*Handler).GetWorkspaceFileHead)

	registry.Query(MethodMeta{
		Name: WorkspaceFilesSearch,
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, (*Handler).GrepWorkspace)

	registry.Query(MethodMeta{
		Name: WorkspaceFilesList,
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, (*Handler).ListWorkspaceFiles)

	registry.Query(MethodMeta{
		Name: WorkspaceFilesRead,
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
			protocol.ErrUnsupportedMime.Error(),
		},
	}, (*Handler).ReadWorkspaceFile)
}
