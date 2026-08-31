package runtimeembedded

import (
	"context"
	"errors"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/authoringcontext"
)

type authoringContextBinding interface {
	ListAgentDocs(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.AgentDoc], error)
	ListRecipes(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.Recipe], error)
}

type authoringContextAdapter struct{ runtime *Runtime }

var _ authoringcontext.Service = (*authoringContextAdapter)(nil)

func (a *authoringContextAdapter) Documents(ctx context.Context, workspace string) ([]authoringcontext.Document, error) {
	r := a.runtime
	query, err := authoringWorkspaceQuery(workspace)
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
	return projectUniqueValues("list agent documents", values, func(value protocol.AgentDoc) authoringcontext.Document {
		return authoringcontext.Document{Path: value.Path, Title: value.Title, Scope: authoringcontext.DocumentScope(value.Scope)}
	}, func(document authoringcontext.Document) string {
		return document.Path
	})
}

func (a *authoringContextAdapter) Recipes(ctx context.Context, workspace string) ([]authoringcontext.Recipe, error) {
	r := a.runtime
	query, err := authoringWorkspaceQuery(workspace)
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
	return projectUniqueValues("list recipes", values, func(value protocol.Recipe) authoringcontext.Recipe {
		return authoringcontext.Recipe{
			Name: value.Name, Description: value.Description, ArgumentHint: value.ArgumentHint,
			Body: value.Body, Scope: authoringcontext.RecipeScope(value.Scope), Source: value.Source,
		}
	}, func(recipe authoringcontext.Recipe) string {
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
