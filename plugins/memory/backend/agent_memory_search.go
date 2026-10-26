package main

import (
	"context"
	json "encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/invopop/jsonschema"
	"github.com/samber/lo"

	toolcontract "github.com/Tangerg/scope/core/tool"

	agentmemory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

const (
	agentMemorySearchDefaultLimit = 8
	agentMemorySearchMaxLimit     = 20
)

type agentMemorySearchRequest struct {
	Query string `json:"query" jsonschema:"minLength=1" jsonschema_description:"Natural-language topic, decision, convention, or user preference to recall from curated memory visible in the current project context."`
	Limit *int   `json:"limit,omitzero" jsonschema:"minimum=1" jsonschema_description:"Maximum memories to return. Defaults to 8."`
}

func (a agentMemorySearchRequest) normalized() (query string, limit int, err error) {
	query = strings.TrimSpace(a.Query)
	if query == "" {
		return "", 0, errors.New("query is required")
	}
	limit = agentMemorySearchDefaultLimit
	if a.Limit != nil {
		limit = *a.Limit
	}
	return query, limit, nil
}

type agentMemorySearch interface {
	Search(ctx context.Context, project, query string, topK int) ([]agentmemory.Item, error)
}

type agentMemorySearcher struct {
	search agentMemorySearch
}

func newAgentMemorySearch(search agentMemorySearch) (toolcontract.Tool, error) {
	if lo.IsNil(search) {
		return nil, errors.New("memory: tool searcher is required")
	}
	return toolcontract.NewFunc[agentMemorySearchRequest, string](agentMemorySearchDefinition(), (&agentMemorySearcher{search: search}).run)
}

func agentMemorySearchDefinition() toolcontract.FuncConfig {
	return toolcontract.FuncConfig{
		Name: "search_memory",
		Description: "Search curated long-term memory for the current project, including durable decisions, " +
			"conventions, and user preferences from earlier work. Use it when needed context is not already in " +
			"the prompt. Use grep for source code.",
	}
}

func (a *agentMemorySearcher) run(ctx context.Context, req agentMemorySearchRequest) (string, error) {
	query, limit, err := req.normalized()
	if err != nil {
		return "", pluginsdk.DefiniteToolFailure(fmt.Errorf("search_memory: %w", err))
	}
	scope, err := pluginsdk.ToolContextFromContext(ctx)
	if err != nil {
		return "", pluginsdk.DefiniteToolFailure(err)
	}
	cwd := scope.CWD
	items, err := a.search.Search(ctx, filepath.Clean(cwd), query, limit)
	if err != nil {
		return "", pluginsdk.DefiniteToolFailure(fmt.Errorf("search_memory: %w", err))
	}
	return agentMemorySearchResults(items).String(), nil
}

type agentMemorySearchResults []agentmemory.Item

func (a agentMemorySearchResults) String() string {
	if len(a) == 0 {
		return "No relevant memories found for this project. It may not have been recorded yet."
	}
	var b strings.Builder
	for i, item := range a {
		content := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(item.Content()), "- "))
		fmt.Fprintf(&b, "%d. %s\n", i+1, content)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (agentMemorySearchRequest) JSONSchemaExtend(schema *jsonschema.Schema) {
	limit, _ := schema.Properties.Get("limit")
	limit.Maximum = json.Number(strconv.Itoa(agentMemorySearchMaxLimit))
}
