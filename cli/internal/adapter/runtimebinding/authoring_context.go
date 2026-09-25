package runtimebinding

import (
	"context"
	"errors"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type authoringContextBinding interface {
	ListAgentDocs(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.AgentDoc], error)
}

type AuthoringContext struct{ runtime *Connection }

func (a *AuthoringContext) Documents(ctx context.Context, workspacePath string) ([]protocol.AgentDoc, error) {
	r := a.runtime
	query, err := authoringWorkspaceQuery(workspacePath)
	if err != nil {
		return nil, err
	}
	page, err := r.authoringContext.ListAgentDocs(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	documents, err := requireCompletePage("list agent documents", page)
	if err != nil {
		return nil, err
	}
	if err := requireUniqueIdentities("list agent documents", documents, func(document protocol.AgentDoc) string {
		return document.Path
	}); err != nil {
		return nil, err
	}
	previousPhase := -1
	for _, document := range documents {
		phase := agentDocumentRenderPhase(document.Scope)
		if phase < previousPhase {
			return nil, runtimeContractViolation(
				"list agent documents returned scope %q after a later render phase",
				document.Scope,
			)
		}
		previousPhase = phase
	}
	return documents, nil
}

func agentDocumentRenderPhase(scope protocol.AgentDocScope) int {
	switch scope {
	case protocol.AgentDocScopeHome:
		return 0
	case protocol.AgentDocScopeProjectRoot:
		return 1
	case protocol.AgentDocScopeCWD:
		return 2
	default:
		return -1
	}
}

func authoringWorkspaceQuery(workspace string) (protocol.WorkspaceQuery, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return protocol.WorkspaceQuery{}, errors.New("authoring context: workspace is empty")
	}
	return protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: workspace}}, nil
}
