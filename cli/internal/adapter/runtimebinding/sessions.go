package runtimebinding

import (
	"context"
	"fmt"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type sessionCatalogBinding interface {
	ListSessions(context.Context, protocol.ListSessionsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.Session], error)
	CreateSession(context.Context, protocol.CreateSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	UpdateSession(context.Context, protocol.UpdateSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	ForkSession(context.Context, protocol.ForkSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	DeleteSession(context.Context, protocol.DeleteSessionRequest, flameruntime.CommandOptions) error
}

func (r *Connection) ListSessions(ctx context.Context, query agent.SessionQuery) (protocol.Page[protocol.Session], error) {
	query, err := query.Normalize()
	if err != nil {
		return protocol.Page[protocol.Session]{}, err
	}
	limit, err := query.PageSize.Rows()
	if err != nil {
		return protocol.Page[protocol.Session]{}, err
	}
	if err := validateRequestCursor("list sessions", query.Cursor); err != nil {
		return protocol.Page[protocol.Session]{}, err
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
		return protocol.Page[protocol.Session]{}, classifyError(err)
	}
	return projectSessionPage(page, query, limit)
}

func projectSessionPage(page *protocol.Page[protocol.Session], query agent.SessionQuery, limit int) (protocol.Page[protocol.Session], error) {
	if page == nil {
		return protocol.Page[protocol.Session]{}, runtimeContractViolation("list sessions returned a nil page")
	}
	if len(page.Data) > limit {
		return protocol.Page[protocol.Session]{}, runtimeContractViolation("list sessions returned %d rows for limit %d", len(page.Data), limit)
	}
	if err := validateContinuationCursor("list sessions", query.Cursor, page.NextCursor); err != nil {
		return protocol.Page[protocol.Session]{}, err
	}
	seen := make(map[string]struct{}, len(page.Data))
	for index, projected := range page.Data {
		if err := protocol.ValidateWireTree(projected); err != nil {
			return protocol.Page[protocol.Session]{}, runtimeContractViolation("list sessions returned an invalid session: %v", err)
		}
		if query.Workspace != "" && projected.Workspace.Ref.Path != query.Workspace {
			return protocol.Page[protocol.Session]{}, runtimeContractViolation(
				"list sessions for workspace %q returned session %q from %q",
				query.Workspace, projected.ID, projected.Workspace.Ref.Path,
			)
		}
		if query.Search != "" && !sessionMatchesSearch(projected, query.Search) {
			return protocol.Page[protocol.Session]{}, runtimeContractViolation(
				"list sessions for search %q returned non-matching session %q",
				query.Search,
				projected.ID,
			)
		}
		if index > 0 {
			previous := page.Data[index-1]
			misordered := projected.Favorite && !previous.Favorite
			if projected.Favorite == previous.Favorite {
				misordered = projected.UpdatedAt.After(previous.UpdatedAt) ||
					(projected.UpdatedAt.Equal(previous.UpdatedAt) && projected.ID > previous.ID)
			}
			if misordered {
				return protocol.Page[protocol.Session]{}, runtimeContractViolation(
					"list sessions returned session %q out of catalog order after %q", projected.ID, previous.ID,
				)
			}
		}
		if _, duplicate := seen[projected.ID]; duplicate {
			return protocol.Page[protocol.Session]{}, runtimeContractViolation("list sessions repeats id %q", projected.ID)
		}
		seen[projected.ID] = struct{}{}
	}
	return *page, nil
}

func sessionMatchesSearch(value protocol.Session, search string) bool {
	search = strings.ToLower(search)
	return strings.Contains(strings.ToLower(value.Title), search) ||
		strings.Contains(strings.ToLower(value.Workspace.Ref.Path), search)
}

func (r *Connection) CreateSession(ctx context.Context, input agent.CreateSession) (protocol.Session, error) {
	if err := input.Validate(); err != nil {
		return protocol.Session{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.Session{}, err
	}
	request := protocol.CreateSessionRequest{Title: input.Title}
	if input.Workspace != "" {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: input.Workspace})
		if resolveErr != nil {
			return protocol.Session{}, fmt.Errorf("create session workspace: %w", resolveErr)
		}
		request.Workspace = &protocol.WorkspaceRef{Path: resolved.Ref.Path}
	}
	created, err := r.sessionCatalog.CreateSession(ctx, request, options)
	return projectSessionResult("create session", "", created, err)
}

func (r *Connection) UpdateSession(ctx context.Context, input agent.UpdateSession) (protocol.Session, error) {
	if err := input.Validate(); err != nil {
		return protocol.Session{}, err
	}
	if input.Workspace != nil {
		if err := r.requireFeature(protocol.FeatureRelocate); err != nil {
			return protocol.Session{}, err
		}
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.Session{}, err
	}
	request := protocol.UpdateSessionRequest{
		SessionID: input.SessionID, ExpectedRevision: input.ExpectedRevision,
		Title: input.Title, Favorite: input.Favorite,
	}
	if input.Model != nil {
		request.Provider = &input.Model.Provider
		request.Model = &input.Model.Model
	}
	if input.Workspace != nil {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: *input.Workspace})
		if resolveErr != nil {
			return protocol.Session{}, fmt.Errorf("update session workspace: %w", resolveErr)
		}
		request.Workspace = &protocol.WorkspaceRef{Path: resolved.Ref.Path}
	}
	updated, err := r.sessionCatalog.UpdateSession(ctx, request, options)
	return projectSessionResult("update session", input.SessionID, updated, err)
}

func (r *Connection) ForkSession(ctx context.Context, input agent.ForkSession) (protocol.Session, error) {
	if err := input.Validate(); err != nil {
		return protocol.Session{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return protocol.Session{}, err
	}
	forked, err := r.sessionCatalog.ForkSession(ctx, protocol.ForkSessionRequest{
		SessionID: input.SessionID, FromRunID: input.FromRunID, Title: input.Title,
	}, options)
	projected, err := projectSessionResult("fork session", "", forked, err)
	if err != nil {
		return protocol.Session{}, err
	}
	if projected.ID == input.SessionID {
		return protocol.Session{}, runtimeContractViolation("fork session returned source id %q", input.SessionID)
	}
	return projected, nil
}

func projectSessionResult(operation, expectedID string, result *protocol.Session, err error) (protocol.Session, error) {
	if err != nil {
		return protocol.Session{}, classifyError(err)
	}
	if result == nil {
		return protocol.Session{}, runtimeContractViolation("%s returned nil", operation)
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return protocol.Session{}, runtimeContractViolation("%s returned an invalid session: %v", operation, err)
	}
	if expectedID != "" && result.ID != expectedID {
		return protocol.Session{}, runtimeContractViolation("%s returned id %q for %q", operation, result.ID, expectedID)
	}
	return *result, nil
}

func (r *Connection) DeleteSession(ctx context.Context, input agent.DeleteSession) error {
	if err := input.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptionsFor(input.CommandID)
	if err != nil {
		return err
	}
	return classifyError(r.sessionCatalog.DeleteSession(ctx, protocol.DeleteSessionRequest{SessionID: input.SessionID}, options))
}
