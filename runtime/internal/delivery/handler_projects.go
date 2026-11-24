package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/project"
)

func wireProjectErr(err error) error {
	switch {
	case errors.Is(err, project.ErrNotFound):
		return protocol.ErrProjectNotFound
	case errors.Is(err, project.ErrRevisionConflict):
		return protocol.NewFailure(protocol.ErrRevisionConflict, "the project changed after it was read", err)
	case errors.Is(err, project.ErrNameRequired), errors.Is(err, project.ErrInstructionsTooLarge), errors.Is(err, project.ErrInvalid):
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	default:
		return err
	}
}

func (s *Handler) ListProjects(ctx context.Context, in protocol.ListProjectsRequest) (*protocol.Page[protocol.Project], error) {
	limit, err := pagination.ParseLimit(in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	page, err := s.projects.ListPage(ctx, in.Archived, in.Cursor, limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	data := make([]protocol.Project, 0, len(page.Rows))
	for _, value := range page.Rows {
		data = append(data, *presentProject(value))
	}
	return protocol.NewPageWithCursor(data, page.NextCursor), nil
}

func (s *Handler) GetProject(ctx context.Context, in protocol.GetProjectRequest) (*protocol.Project, error) {
	id, err := wireIdentity("projectId", in.ProjectID, identity.ParseProject)
	if err != nil {
		return nil, err
	}
	value, err := s.projects.Get(ctx, id)
	if err != nil {
		return nil, wireProjectErr(err)
	}
	return presentProject(value), nil
}

func (s *Handler) CreateProject(ctx context.Context, in protocol.CreateProjectRequest) (*protocol.Project, error) {
	var created project.Project
	if err := s.createProject(ctx, in, directReply(&created)); err != nil {
		return nil, err
	}
	return presentProject(created), nil
}

func (s *Handler) commitCreateProject(ctx context.Context, in protocol.CreateProjectRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.createProject(ctx, in, outcome.project()))
}

func (s *Handler) createProject(ctx context.Context, in protocol.CreateProjectRequest, complete operation.Reply[project.Project]) error {
	return wireProjectErr(s.projects.Create(ctx, in.Name, in.Instructions, complete))
}

func (s *Handler) UpdateProject(ctx context.Context, in protocol.UpdateProjectRequest) (*protocol.Project, error) {
	var updated project.Project
	if err := s.updateProject(ctx, in, directReply(&updated)); err != nil {
		return nil, err
	}
	return presentProject(updated), nil
}

func (s *Handler) commitUpdateProject(ctx context.Context, in protocol.UpdateProjectRequest, outcome atomicOutcome) error {
	return outcome.settleRefusal(ctx, s.updateProject(ctx, in, outcome.project()))
}

func (s *Handler) updateProject(ctx context.Context, in protocol.UpdateProjectRequest, complete operation.Reply[project.Project]) error {
	id, err := wireIdentity("projectId", in.ProjectID, identity.ParseProject)
	if err != nil {
		return err
	}
	return wireProjectErr(s.projects.Update(ctx, id, in.ExpectedRevision, project.Patch{
		Name: in.Name, Instructions: in.Instructions, Archived: in.Archived,
	}, complete))
}

func presentProject(value project.Project) *protocol.Project {
	snapshot := value.Snapshot()
	return &protocol.Project{
		ID: snapshot.ID.String(), Name: snapshot.Name, Instructions: snapshot.Instructions,
		Archived: snapshot.Archived, CreatedAt: snapshot.CreatedAt, UpdatedAt: snapshot.UpdatedAt,
		Revision: snapshot.Revision,
	}
}
