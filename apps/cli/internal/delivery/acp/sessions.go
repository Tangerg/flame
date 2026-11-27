package acp

import (
	"context"
	"time"

	"github.com/Tangerg/acp"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (a agent) newSession(ctx context.Context, _ *acp.AgentConn, request *acp.NewSessionRequest) (*acp.NewSessionResponse, error) {
	if err := checkSessionServers(request.McpServers); err != nil {
		return nil, err
	}
	session, err := a.runtime.CreateSession(ctx, conversation.CreateSession{Workspace: request.Cwd})
	if err != nil {
		return nil, runtimeError(err)
	}
	return &acp.NewSessionResponse{SessionID: acp.SessionID(session.ID)}, nil
}

func (a agent) listSessions(ctx context.Context, _ *acp.AgentConn, request *acp.ListSessionsRequest) (*acp.ListSessionsResponse, error) {
	cursor, _ := request.Cursor.Get()
	query := conversation.SessionQuery{Cursor: cursor, PageSize: conversation.DefaultPageSize()}
	if cwd, exists := request.Cwd.Get(); exists {
		resolved, err := a.runtime.Resolve(ctx, workspace.ResolveRequest{Path: cwd})
		if err != nil {
			return nil, runtimeError(err)
		}
		query.Workspace = resolved.Path
	}
	page, err := a.runtime.ListSessions(ctx, query)
	if err != nil {
		return nil, runtimeError(err)
	}
	result := &acp.ListSessionsResponse{Sessions: make([]acp.SessionInfo, 0, len(page.Items))}
	for _, session := range page.Items {
		info := acp.SessionInfo{
			SessionID: acp.SessionID(session.ID), Cwd: session.Workspace.Path,
			UpdatedAt: acp.OptValue(session.UpdatedAt.Format(time.RFC3339Nano)),
		}
		if session.Title != "" {
			info.Title = acp.OptValue(session.Title)
		}
		result.Sessions = append(result.Sessions, info)
	}
	if page.NextCursor != "" {
		result.NextCursor = acp.OptValue(page.NextCursor)
	}
	return result, nil
}

func (a agent) loadSession(ctx context.Context, session *acp.AgentSession, request *acp.LoadSessionRequest) (*acp.LoadSessionResponse, error) {
	snapshot, err := a.openSession(ctx, session.ID(), request.Cwd, request.McpServers)
	if err != nil {
		return nil, err
	}
	if err := newProjection(ctx, session).replay(snapshot); err != nil {
		return nil, runtimeError(err)
	}
	return &acp.LoadSessionResponse{}, nil
}

func (a agent) resumeSession(ctx context.Context, session *acp.AgentSession, request *acp.ResumeSessionRequest) (*acp.ResumeSessionResponse, error) {
	if _, err := a.openSession(ctx, session.ID(), request.Cwd, request.McpServers); err != nil {
		return nil, err
	}
	return &acp.ResumeSessionResponse{}, nil
}

func (a agent) openSession(ctx context.Context, id acp.SessionID, cwd string, servers []acp.McpServer) (conversation.SessionSnapshot, error) {
	if err := checkSessionServers(servers); err != nil {
		return conversation.SessionSnapshot{}, err
	}
	if err := protocol.ValidateSessionID(string(id)); err != nil {
		return conversation.SessionSnapshot{}, invalidParams("invalid session identifier")
	}
	resolved, err := a.runtime.Resolve(ctx, workspace.ResolveRequest{Path: cwd})
	if err != nil {
		return conversation.SessionSnapshot{}, runtimeError(err)
	}
	snapshot, err := a.runtime.GetSession(ctx, string(id))
	if err != nil {
		return conversation.SessionSnapshot{}, runtimeError(err)
	}
	if resolved.Path != snapshot.Session.Workspace.Path {
		return conversation.SessionSnapshot{}, invalidParams("cwd does not match the Flame session workspace")
	}
	return snapshot, nil
}

func checkSessionServers(servers []acp.McpServer) error {
	if len(servers) != 0 {
		return invalidParams("session-supplied MCP servers are unsupported; configure them in Flame")
	}
	return nil
}
