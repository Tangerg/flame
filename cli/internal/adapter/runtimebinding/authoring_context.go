package runtimebinding

import (
	"context"
	"errors"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type authoringContextBinding interface {
	ListAgentDocs(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.AgentDoc], error)
	ListRecipes(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.Recipe], error)
}

type authoringContextAdapter struct{ runtime *Connection }

var _ workspace.AuthoringContextService = (*authoringContextAdapter)(nil)

func (a *authoringContextAdapter) Documents(ctx context.Context, workspacePath string) ([]workspace.AuthoringDocument, error) {
	r := a.runtime
	query, err := authoringWorkspaceQuery(workspacePath)
	if err != nil {
		return nil, err
	}
	page, err := r.authoringContext.ListAgentDocs(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list agent documents", page)
	if err != nil {
		return nil, err
	}
	return projectUniqueValues("list agent documents", values, func(value protocol.AgentDoc) workspace.AuthoringDocument {
		return workspace.AuthoringDocument{Path: value.Path, Title: value.Title, Scope: workspace.AuthoringDocumentScope(value.Scope)}
	}, func(document workspace.AuthoringDocument) string {
		return document.Path
	})
}

func (a *authoringContextAdapter) Recipes(ctx context.Context, workspacePath string) ([]workspace.AuthoringRecipe, error) {
	r := a.runtime
	query, err := authoringWorkspaceQuery(workspacePath)
	if err != nil {
		return nil, err
	}
	page, err := r.authoringContext.ListRecipes(ctx, query, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list recipes", page)
	if err != nil {
		return nil, err
	}
	return projectUniqueValues("list recipes", values, func(value protocol.Recipe) workspace.AuthoringRecipe {
		return workspace.AuthoringRecipe{
			Name: value.Name, Description: value.Description, ArgumentHint: value.ArgumentHint,
			Body: value.Body, Scope: workspace.AuthoringRecipeScope(value.Scope), Source: value.Source,
		}
	}, func(recipe workspace.AuthoringRecipe) string {
		return recipe.Name
	})
}

func authoringWorkspaceQuery(workspace string) (protocol.WorkspaceQuery, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return protocol.WorkspaceQuery{}, errors.New("authoring context: workspace is empty")
	}
	return protocol.WorkspaceQuery{Workspace: protocol.WorkspaceRef{Path: workspace}}, nil
}
