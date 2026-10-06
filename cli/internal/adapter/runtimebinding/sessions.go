package runtimebinding

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type sessionCatalogBinding interface {
	ListSessions(context.Context, protocol.ListSessionsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.Session], error)
	CreateSession(context.Context, protocol.CreateSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	UpdateSession(context.Context, protocol.UpdateSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	ForkSession(context.Context, protocol.ForkSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error)
	DeleteSession(context.Context, protocol.DeleteSessionRequest, flameruntime.CommandOptions) error
}

func (r *Connection) ListSessions(ctx context.Context, query conversation.SessionQuery) (conversation.SessionPage, error) {
	query, err := query.Normalize()
	if err != nil {
		return conversation.SessionPage{}, err
	}
	limit, err := query.PageSize.Rows()
	if err != nil {
		return conversation.SessionPage{}, err
	}
	if err := validateRequestCursor("list sessions", query.Cursor); err != nil {
		return conversation.SessionPage{}, err
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
		return conversation.SessionPage{}, classifyError(err)
	}
	return projectSessionPage(page, query, limit)
}

func projectSessionPage(page *protocol.Page[protocol.Session], query conversation.SessionQuery, limit int) (conversation.SessionPage, error) {
	if page == nil {
		return conversation.SessionPage{}, runtimeContractViolation("list sessions returned a nil page")
	}
	if len(page.Data) > limit {
		return conversation.SessionPage{}, runtimeContractViolation("list sessions returned %d rows for limit %d", len(page.Data), limit)
	}
	if err := validateContinuationCursor("list sessions", query.Cursor, page.NextCursor); err != nil {
		return conversation.SessionPage{}, err
	}
	result := conversation.SessionPage{Items: make([]conversation.Session, 0, len(page.Data)), NextCursor: page.NextCursor}
	for _, value := range page.Data {
		projected := projectSession(value)
		if query.Workspace != "" && projected.Workspace.Path != query.Workspace {
			return conversation.SessionPage{}, runtimeContractViolation(
				"list sessions for workspace %q returned session %q from %q",
				query.Workspace, projected.ID, projected.Workspace.Path,
			)
		}
		if query.Search != "" && !sessionMatchesSearch(projected, query.Search) {
			return conversation.SessionPage{}, runtimeContractViolation(
				"list sessions for search %q returned non-matching session %q",
				query.Search,
				projected.ID,
			)
		}
		if len(result.Items) != 0 {
			previous := result.Items[len(result.Items)-1]
			misordered := projected.Favorite && !previous.Favorite
			if projected.Favorite == previous.Favorite {
				misordered = projected.UpdatedAt.After(previous.UpdatedAt) ||
					(projected.UpdatedAt.Equal(previous.UpdatedAt) && projected.ID > previous.ID)
			}
			if misordered {
				return conversation.SessionPage{}, runtimeContractViolation(
					"list sessions returned session %q out of catalog order after %q", projected.ID, previous.ID,
				)
			}
		}
		result.Items = append(result.Items, projected)
	}
	if err := requireUniqueIdentities("list sessions", result.Items, func(value conversation.Session) string {
		return value.ID
	}); err != nil {
		return conversation.SessionPage{}, err
	}
	return result, nil
}

func sessionMatchesSearch(value conversation.Session, search string) bool {
	search = strings.ToLower(search)
	return strings.Contains(strings.ToLower(value.Title), search) ||
		strings.Contains(strings.ToLower(value.Workspace.Path), search)
}

func projectSession(value protocol.Session) conversation.Session {
	return conversation.Session{
		ID: value.ID, Title: value.Title, Status: value.Status,
		Provider: value.Provider, Model: value.Model, ReasoningEffort: value.ReasoningEffort,
		Workspace: projectWorkspace(value.Workspace), CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		Favorite: value.Favorite, Revision: value.Revision,
	}
}

func (r *Connection) CreateSession(ctx context.Context, input conversation.CreateSession) (conversation.Session, error) {
	if err := input.Validate(); err != nil {
		return conversation.Session{}, err
	}
	options := r.commandOptions()
	validated := input
	if input.Workspace != "" {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: input.Workspace})
		if resolveErr != nil {
			return conversation.Session{}, fmt.Errorf("create session workspace: %w", resolveErr)
		}
		validated.Workspace = resolved.Path
	}
	request := protocol.CreateSessionRequest{Title: input.Title}
	if validated.Workspace != "" {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	created, err := r.sessionCatalog.CreateSession(ctx, request, options)
	return projectSessionResult("create session", "", created, err)
}

func (r *Connection) UpdateSession(ctx context.Context, input conversation.UpdateSession) (conversation.Session, error) {
	if err := input.Validate(); err != nil {
		return conversation.Session{}, err
	}
	if input.Workspace != nil {
		if err := r.requireFeature(protocol.FeatureRelocate); err != nil {
			return conversation.Session{}, err
		}
	}
	options := r.commandOptions()
	validated := input
	if input.Workspace != nil {
		resolved, resolveErr := r.Resolve(ctx, workspace.ResolveRequest{Path: *input.Workspace})
		if resolveErr != nil {
			return conversation.Session{}, fmt.Errorf("update session workspace: %w", resolveErr)
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
	return projectSessionResult("update session", input.SessionID, updated, err)
}

func (r *Connection) ForkSession(ctx context.Context, input conversation.ForkSession) (conversation.Session, error) {
	if err := input.Validate(); err != nil {
		return conversation.Session{}, err
	}
	options := r.commandOptions()
	forked, err := r.sessionCatalog.ForkSession(ctx, protocol.ForkSessionRequest{
		SessionID: input.SessionID, FromRunID: input.FromRunID, Title: input.Title,
	}, options)
	return projectSessionResult("fork session", "", forked, err)
}

func projectSessionResult(operation, expectedID string, result *protocol.Session, err error) (conversation.Session, error) {
	if err != nil {
		return conversation.Session{}, classifyError(err)
	}
	if result == nil {
		return conversation.Session{}, runtimeContractViolation("%s returned nil", operation)
	}
	projected := projectSession(*result)
	if err := requireIdentity(operation, projected.ID, expectedID); err != nil {
		return conversation.Session{}, err
	}
	return projected, nil
}

func (r *Connection) DeleteSession(ctx context.Context, input conversation.DeleteSession) error {
	if err := input.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptionsFor(input.CommandID)
	if err != nil {
		return err
	}
	return classifyError(r.sessionCatalog.DeleteSession(ctx, protocol.DeleteSessionRequest{SessionID: input.SessionID}, options))
}
