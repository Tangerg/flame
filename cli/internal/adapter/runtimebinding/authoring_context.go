package runtimebinding

import (
	"context"
	"errors"
	"slices"
	"strings"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type authoringContextBinding interface {
	ListAgentDocs(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.AgentDoc], error)
	ListRecipes(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.Recipe], error)
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
	values, err := requireCompletePage("list agent documents", page)
	if err != nil {
		return nil, err
	}
	documents := slices.Clone(values)
	seen := make(map[string]struct{}, len(documents))
	for index, document := range documents {
		if err := document.ValidateWire(); err != nil {
			return nil, runtimeContractViolation("list agent documents item %d is invalid: %v", index+1, err)
		}
		if _, duplicate := seen[document.Path]; duplicate {
			return nil, runtimeContractViolation("list agent documents repeats %q", document.Path)
		}
		seen[document.Path] = struct{}{}
	}
	return documents, nil
}

func (a *AuthoringContext) Recipes(ctx context.Context, workspacePath string) ([]workspace.AuthoringRecipe, error) {
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
	return projectUniqueValuesFallible("list recipes", values, func(value protocol.Recipe) (workspace.AuthoringRecipe, error) {
		if err := protocol.ValidateWireTree(value); err != nil {
			return workspace.AuthoringRecipe{}, err
		}
		return workspace.AuthoringRecipe{
			Name: value.Name, Description: value.Description, ArgumentHint: value.ArgumentHint,
			Body: value.Body, Scope: value.Scope, Source: value.Source,
		}, nil
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
