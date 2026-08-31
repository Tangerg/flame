package runtimeembedded

import (
	"context"
	"fmt"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/agent"
	"github.com/Tangerg/flame/cli/internal/runtimeprofile"
	"github.com/Tangerg/flame/cli/internal/workspace"
)

type sessionCatalogBinding interface {
	ListSessions(context.Context, protocol.ListSessionsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.Session], error)
	CreateSession(context.Context, protocol.CreateSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	UpdateSession(context.Context, protocol.UpdateSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	ForkSession(context.Context, protocol.ForkSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	DeleteSession(context.Context, protocol.DeleteSessionRequest, flameruntime.CommandOptions) error
}

func (r *Runtime) ListSessions(ctx context.Context, query agent.SessionQuery) (agent.SessionPage, error) {
	query, err := query.Normalize()
	if err != nil {
		return agent.SessionPage{}, err
	}
	limit, err := query.PageSize.Rows()
	if err != nil {
		return agent.SessionPage{}, err
	}
	if err := validateRequestCursor("list sessions", query.Cursor); err != nil {
		return agent.SessionPage{}, err
	}
	request := protocol.ListSessionsRequest{
		PageQuery: protocol.PageQuery{Limit: protocolPositiveInt(limit), Cursor: query.Cursor},
		Search:    query.Search,
	}
	if query.Workspace != "" {
		request.Workspace = &protocol.WorkspaceRef{Path: query.Workspace}
	}
	page, err := r.sessionCatalog.ListSessions(ctx, request, r.callOptions())
	if err != nil {
		return agent.SessionPage{}, classifyError(err)
	}
	return projectSessionPage(page, query.Cursor, limit)
}

func projectSessionPage(page *protocol.Page[protocol.Session], cursor string, limit int) (agent.SessionPage, error) {
	if page == nil {
		return agent.SessionPage{}, runtimeContractViolation("list sessions returned a nil page")
	}
	if len(page.Data) > limit {
		return agent.SessionPage{}, runtimeContractViolation("list sessions returned %d rows for limit %d", len(page.Data), limit)
	}
	if err := validateContinuationCursor("list sessions", cursor, page.NextCursor); err != nil {
		return agent.SessionPage{}, err
	}
	result := agent.SessionPage{Items: make([]agent.Session, 0, len(page.Data)), NextCursor: page.NextCursor}
	for _, value := range page.Data {
		projected, err := projectSession(value)
		if err != nil {
			return agent.SessionPage{}, runtimeContractViolation("list sessions returned an invalid session: %v", err)
		}
		result.Items = append(result.Items, projected)
	}
	if err := result.Validate(); err != nil {
		return agent.SessionPage{}, runtimeContractViolation("list sessions returned an invalid projection: %v", err)
	}
	return result, nil
}

func projectSession(value protocol.Session) (agent.Session, error) {
	projectedWorkspace, err := projectWorkspace(value.Workspace)
	if err != nil {
		return agent.Session{}, fmt.Errorf("runtime session %q: %w", value.ID, err)
	}
	status := agent.SessionStatus(value.Status)
	projected := agent.Session{
		ID: value.ID, Title: value.Title, Status: status, Provider: value.Provider, Model: value.Model,
		Workspace: projectedWorkspace, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		Favorite: value.Favorite, Revision: value.Revision,
	}
	if err := projected.Validate(); err != nil {
		return agent.Session{}, fmt.Errorf("runtime session %q: %w", value.ID, err)
	}
	return projected, nil
}

func (r *Runtime) CreateSession(ctx context.Context, input agent.CreateSession) (agent.Session, error) {
	if err := input.Validate(); err != nil {
		return agent.Session{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.Session{}, err
	}
	validated := input
	if input.Workspace != "" {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: input.Workspace})
		if resolveErr != nil {
			return agent.Session{}, fmt.Errorf("create session workspace: %w", resolveErr)
		}
		validated.Workspace = resolved.Path
	}
	request := protocol.CreateSessionRequest{Title: input.Title}
	if validated.Workspace != "" {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	created, err := r.sessionCatalog.CreateSession(ctx, request, options)
	projected, err := projectSessionResult("create session", "", created, err)
	if err != nil {
		return agent.Session{}, err
	}
	if err := validated.ValidateResult(projected); err != nil {
		return agent.Session{}, runtimeContractViolation("create session returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Runtime) UpdateSession(ctx context.Context, input agent.UpdateSession) (agent.Session, error) {
	if err := input.Validate(); err != nil {
		return agent.Session{}, err
	}
	if input.Workspace != nil {
		if err := r.requireFeature(runtimeprofile.FeatureRelocate); err != nil {
			return agent.Session{}, err
		}
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.Session{}, err
	}
	validated := input
	if input.Workspace != nil {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: *input.Workspace})
		if resolveErr != nil {
			return agent.Session{}, fmt.Errorf("update session workspace: %w", resolveErr)
		}
		validated.Workspace = &resolved.Path
	}
	request := protocol.UpdateSessionRequest{
		SessionID: input.SessionID, ExpectedRevision: input.ExpectedRevision,
		Title: input.Title, Favorite: input.Favorite,
	}
	if input.Model != nil {
		request.Provider = &input.Model.Provider
		request.Model = &input.Model.Model
	}
	if validated.Workspace != nil {
		request.Workspace = &protocol.WorkspaceRef{Path: *validated.Workspace}
	}
	updated, err := r.sessionCatalog.UpdateSession(ctx, request, options)
	projected, err := projectSessionResult("update session", input.SessionID, updated, err)
	if err != nil {
		return agent.Session{}, err
	}
	if err := validated.ValidateResult(projected); err != nil {
		return agent.Session{}, runtimeContractViolation("update session returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Runtime) ForkSession(ctx context.Context, input agent.ForkSession) (agent.Session, error) {
	if err := input.Validate(); err != nil {
		return agent.Session{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return agent.Session{}, err
	}
	forked, err := r.sessionCatalog.ForkSession(ctx, protocol.ForkSessionRequest{
		SessionID: input.SessionID, FromRunID: input.FromRunID, Title: input.Title,
	}, options)
	projected, err := projectSessionResult("fork session", "", forked, err)
	if err != nil {
		return agent.Session{}, err
	}
	if err := input.ValidateResult(projected); err != nil {
		return agent.Session{}, runtimeContractViolation("fork session returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func projectSessionResult(operation, expectedID string, result *protocol.Session, err error) (agent.Session, error) {
	if err != nil {
		return agent.Session{}, classifyError(err)
	}
	if result == nil {
		return agent.Session{}, runtimeContractViolation("%s returned nil", operation)
	}
	projected, err := projectSession(*result)
	if err != nil {
		return agent.Session{}, runtimeContractViolation("%s returned an invalid session: %v", operation, err)
	}
	if expectedID != "" && projected.ID != expectedID {
		return agent.Session{}, runtimeContractViolation("%s returned id %q for %q", operation, projected.ID, expectedID)
	}
	return projected, nil
}

func (r *Runtime) DeleteSession(ctx context.Context, input agent.DeleteSession) error {
	if err := input.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptionsFor(input.CommandID)
	if err != nil {
		return err
	}
	return classifyError(r.sessionCatalog.DeleteSession(ctx, protocol.DeleteSessionRequest{SessionID: input.SessionID}, options))
}
