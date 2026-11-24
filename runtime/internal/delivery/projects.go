package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	projectsList   Name = "projects.list"
	projectsGet    Name = "projects.get"
	projectsCreate Name = "projects.create"
	projectsUpdate Name = "projects.update"
)

func registerProjects(registry *Registry) {
	registry.query(MethodMeta{Name: projectsList, GoName: "ListProjects"},
		func(service interface {
			ListProjects(context.Context, protocol.ListProjectsRequest) (*protocol.Page[protocol.Project], error)
		}, ctx context.Context, request protocol.ListProjectsRequest) (*protocol.Page[protocol.Project], error) {
			return service.ListProjects(ctx, request)
		})

	registry.query(MethodMeta{
		Name: projectsGet, GoName: "GetProject",
		Errors: []string{protocol.ErrProjectNotFound.Error()},
	}, func(service interface {
		GetProject(context.Context, protocol.GetProjectRequest) (*protocol.Project, error)
	}, ctx context.Context, request protocol.GetProjectRequest) (*protocol.Project, error) {
		return service.GetProject(ctx, request)
	})

	registry.command(MethodMeta{Name: projectsCreate, GoName: "CreateProject"},
		func(service interface {
			CreateProject(context.Context, protocol.CreateProjectRequest) (*protocol.Project, error)
		}, ctx context.Context, request protocol.CreateProjectRequest) (*protocol.Project, error) {
			return service.CreateProject(ctx, request)
		}).withOwnedReplay(atomicReplay((interface {
		commitCreateProject(context.Context, protocol.CreateProjectRequest, atomicOutcome) error
	}).commitCreateProject))

	registry.command(MethodMeta{
		Name: projectsUpdate, GoName: "UpdateProject",
		Errors: []string{protocol.ErrProjectNotFound.Error(), protocol.ErrRevisionConflict.Error()},
	}, func(service interface {
		UpdateProject(context.Context, protocol.UpdateProjectRequest) (*protocol.Project, error)
	}, ctx context.Context, request protocol.UpdateProjectRequest) (*protocol.Project, error) {
		return service.UpdateProject(ctx, request)
	}).withOwnedReplay(atomicReplay((interface {
		commitUpdateProject(context.Context, protocol.UpdateProjectRequest, atomicOutcome) error
	}).commitUpdateProject))
}
