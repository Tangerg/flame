package toolset_test

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type mcpTool struct {
	name, desc, server, remote, result string
}

type discoveryDecorator struct{ toolcontract.Tool }

func (d discoveryDecorator) MCPSource() (mcp.CapturedSource, bool) { return mcp.Source(d.Tool) }

func (m mcpTool) Definition() toolcontract.Definition {
	return toolcontract.Definition{
		Name:        m.name,
		Description: m.desc,
		InputSchema: jsontext.Value(`{"type":"object"}`),
	}
}

func (m mcpTool) Call(context.Context, toolcontract.Invocation) (toolcontract.Result, error) {
	if m.result == "" {
		return toolcontract.Result{Output: toolcontract.NewTextOutput("ok")}, nil
	}
	return toolcontract.Result{Output: toolcontract.NewTextOutput(m.result)}, nil
}

func (m mcpTool) MCPToolIdentity() (string, string) { return m.server, m.remote }

func (m mcpTool) SourceConfig() mcp.ServerConfig {
	return mcp.ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName(m.server)}}
}

func catalog() []toolcontract.Tool {
	return []toolcontract.Tool{
		mcpTool{name: "linear_create_issue", desc: "Create a Linear issue", server: "linear", remote: "create_issue"},
		mcpTool{name: "linear_list_issues", desc: "List Linear issues", server: "linear", remote: "list_issues"},
		mcpTool{name: "slack_send_message", desc: "Send a Slack message", server: "slack", remote: "send_message"},
		mcpTool{name: "github_open_pr", desc: "Open a GitHub pull request", server: "github", remote: "open_pr"},
	}
}

func call(t *testing.T, tool *toolset.Discovery, query string) string {
	t.Helper()
	args, _ := json.Marshal(map[string]any{"query": query})
	ctx := context.Background()
	out, err := callTextTool(ctx, tool, string(args))
	if err != nil {
		t.Fatalf("Call(%q): %v", query, err)
	}
	return out
}

func newSearch(t *testing.T, tools []toolcontract.Tool) *toolset.Discovery {
	t.Helper()
	search, err := toolset.NewDiscovery(tools)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return search
}

func TestNewEmptyReturnsNil(t *testing.T) {
	search, err := toolset.NewDiscovery(nil)
	if err != nil {
		t.Fatalf("New(nil): %v", err)
	}
	if search != nil {
		t.Fatal("New(nil) should return a nil tool — nothing to search")
	}
}

func TestKeywordSearchRanksByName(t *testing.T) {
	tool := newSearch(t, catalog())
	out := call(t, tool, "create issue")
	if !strings.Contains(out, "linear_create_issue") {
		t.Fatalf("expected linear_create_issue loaded, got:\n%s", out)
	}
	if strings.Contains(out, "slack_send_message") {
		t.Fatalf("unrelated tool loaded, got:\n%s", out)
	}
}

func TestRequiredTermExcludesNonMatches(t *testing.T) {
	tool := newSearch(t, catalog())
	out := call(t, tool, "+slack message")
	if !strings.Contains(out, "slack_send_message") {
		t.Fatalf("expected slack_send_message, got:\n%s", out)
	}
	if strings.Contains(out, "linear") || strings.Contains(out, "github") {
		t.Fatalf("+slack should exclude non-slack tools, got:\n%s", out)
	}
}

func TestSelectByExactName(t *testing.T) {
	tool := newSearch(t, catalog())
	out := call(t, tool, "select:github_open_pr,linear_list_issues")
	if !strings.Contains(out, "github_open_pr") || !strings.Contains(out, "linear_list_issues") {
		t.Fatalf("select did not load both named tools:\n%s", out)
	}
	if strings.Contains(out, "slack_send_message") {
		t.Fatalf("select loaded an unnamed tool:\n%s", out)
	}
}

func TestSelectDropsUnknownNames(t *testing.T) {
	tool := newSearch(t, catalog())
	out := call(t, tool, "select:does_not_exist")
	if !strings.Contains(out, "No tools matched") {
		t.Fatalf("unknown select should report no match, got:\n%s", out)
	}
}

// TestRoundRobinSpreadsAcrossServers: a query that matches every tool equally
// well must not let one server monopolize the (limited) result window.
func TestRoundRobinSpreadsAcrossServers(t *testing.T) {
	// Six tools, three each on two servers, all matching "issue".
	many := []toolcontract.Tool{
		mcpTool{name: "alpha_issue_a", desc: "issue", server: "alpha", remote: "a"},
		mcpTool{name: "alpha_issue_b", desc: "issue", server: "alpha", remote: "b"},
		mcpTool{name: "alpha_issue_c", desc: "issue", server: "alpha", remote: "c"},
		mcpTool{name: "beta_issue_a", desc: "issue", server: "beta", remote: "a"},
		mcpTool{name: "beta_issue_b", desc: "issue", server: "beta", remote: "b"},
		mcpTool{name: "beta_issue_c", desc: "issue", server: "beta", remote: "c"},
	}
	for index := range many {
		many[index] = discoveryDecorator{Tool: many[index]}
	}
	tool := newSearch(t, many)
	args, _ := json.Marshal(map[string]any{"query": "issue", "limit": 2})
	ctx := context.Background()
	out, err := callTextTool(ctx, tool, string(args))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "alpha_") || !strings.Contains(out, "beta_") {
		t.Fatalf("limit=2 over two servers should include both servers, got:\n%s", out)
	}
}

func TestDeferredToolNames(t *testing.T) {
	tool := newSearch(t, catalog())
	names := tool.DeferredToolNames()
	if len(names) != 4 {
		t.Fatalf("DeferredToolNames = %v, want 4 names", names)
	}
	// Mutating the returned slice must not corrupt internal state.
	names[0] = "mutated"
	if tool.DeferredToolNames()[0] == "mutated" {
		t.Fatal("DeferredToolNames leaked internal slice")
	}
}

func TestDescriptionIsIndependentOfCatalog(t *testing.T) {
	small := newSearch(t, catalog()[:1]).Definition()
	full := newSearch(t, catalog()).Definition()
	if small.Description != full.Description || string(small.InputSchema) != string(full.InputSchema) {
		t.Fatalf("search_tools declaration depends on the deferred catalog:\n%s\n---\n%s", small.Description, full.Description)
	}
	for _, name := range []string{"linear_create_issue", "slack_send_message", "github_open_pr"} {
		if strings.Contains(full.Description, name) {
			t.Fatalf("description names deferred Tool %q:\n%s", name, full.Description)
		}
	}
}

func TestCatalogListsSelectableNamesInStableOrderWithoutSchemas(t *testing.T) {
	tools := catalog()
	reversed := make([]toolcontract.Tool, len(tools))
	for index, tool := range tools {
		reversed[len(tools)-1-index] = tool
	}
	search := newSearch(t, tools)
	rendered := search.Catalog()
	if rendered != newSearch(t, reversed).Catalog() {
		t.Fatalf("catalog depends on input order:\n%s", rendered)
	}
	position := -1
	for _, name := range []string{"github_open_pr", "linear_create_issue", "linear_list_issues", "slack_send_message"} {
		next := strings.Index(rendered, name)
		if next <= position {
			t.Fatalf("catalog does not list %q in source-then-name order:\n%s", name, rendered)
		}
		position = next
	}
	for _, name := range search.DeferredToolNames() {
		if !strings.Contains(call(t, search, "select:"+name), name) {
			t.Fatalf("catalog name %q is not selectable", name)
		}
	}
	if strings.Contains(rendered, "input_schema") || strings.Contains(rendered, `"type":"object"`) {
		t.Fatalf("catalog leaked schemas (defeats deferral):\n%s", rendered)
	}
	var empty *toolset.Discovery
	if empty.Catalog() != "" {
		t.Fatal("an empty deferred set rendered a catalog")
	}
}

func TestLargeDeferredDirectoryIsBoundedWithoutShrinkingDiscovery(t *testing.T) {
	var tools []toolcontract.Tool
	for index := range 1000 {
		name := fmt.Sprintf("catalog_tool_%04d", index)
		tools = append(tools, mcpTool{name: name, desc: fmt.Sprintf("unique-capability-%04d", index), server: "catalog", remote: name})
	}
	search := newSearch(t, tools)
	rendered := search.Catalog()
	if len(rendered) > 4096 || !strings.Contains(rendered, "omitted from this directory") || strings.Contains(rendered, "catalog_tool_0999") {
		t.Fatalf("large directory was not truthfully bounded: %d bytes", len(rendered))
	}
	if len(search.DeferredToolNames()) != len(tools) || !strings.Contains(call(t, search, "unique-capability-0999"), "catalog_tool_0999") ||
		!strings.Contains(call(t, search, "select:catalog_tool_0999"), "catalog_tool_0999") {
		t.Fatal("bounded directory removed executable discovery authority")
	}
	var selected []string
	for index := range 21 {
		selected = append(selected, fmt.Sprintf("catalog_tool_%04d", index))
	}
	arguments, _ := json.Marshal(map[string]string{"query": "select:" + strings.Join(selected, ",")})
	if _, err := callTextTool(t.Context(), search, string(arguments)); err == nil {
		t.Fatal("exact selection bypassed the discovery promotion bound")
	}
}

func TestEmptyQueryErrors(t *testing.T) {
	tool := newSearch(t, catalog())
	_, err := callTextTool(context.Background(), tool, `{"query":"  "}`)
	if err == nil {
		t.Fatal("blank query should error so the model retries")
	}
}

func TestArgumentsAreStrictAndBounded(t *testing.T) {
	tool := newSearch(t, catalog())
	for _, arguments := range []string{
		`{"query":"issue","unknown":true}`,
		`{"query":"issue","limit":0}`,
		`{"query":"issue","limit":21}`,
		`{}`,
	} {
		if _, err := callTextTool(context.Background(), tool, arguments); err == nil {
			t.Errorf("Call(%s) succeeded, want contract validation error", arguments)
		}
	}
}

func TestDiscoveryRejectsUndecodableIntegersAtAdmission(t *testing.T) {
	binding, err := toolcontract.Bind(newSearch(t, catalog()))
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{
		`{"query":"issue","limit":1e1}`,
		`{"query":"issue","limit":10.0}`,
	} {
		if _, err := binding.Contract().Prepare((chat.ToolCall{ID: "search", Name: "search_tools", Arguments: arguments}).Request(), toolcontract.Admission{}); !errors.Is(err, toolcontract.ErrInvalidInvocation) {
			t.Errorf("discovery admitted %s: %v", arguments, err)
		}
	}
	if _, err := binding.Contract().Prepare((chat.ToolCall{ID: "search", Name: "search_tools", Arguments: `{"query":"issue","limit":10}`}).Request(), toolcontract.Admission{}); err != nil {
		t.Fatalf("discovery rejected valid integer: %v", err)
	}
}

func discover(t *testing.T, search *toolset.Discovery, arguments string) (toolcontract.Invocation, toolcontract.Result) {
	t.Helper()
	binding, err := toolcontract.Bind(search)
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := binding.Contract().Prepare((chat.ToolCall{ID: "search", Name: "search_tools", Arguments: arguments}).Request(), toolcontract.Admission{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := binding.Call(t.Context(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	return invocation, result
}

func TestAdvertisementUsesExactExecutedMatches(t *testing.T) {
	search := newSearch(t, catalog())
	original, _ := discover(t, search, `{"query":"select:github_open_pr"}`)
	_, result := discover(t, search, `{"query":"select:linear_list_issues,slack_send_message"}`)
	advertised, err := search.AdvertiseDeferred(t.Context(), original, result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(advertised, ",") != "linear_list_issues,slack_send_message" {
		t.Fatalf("advertised = %v, want the executed result after an argument rewrite", advertised)
	}
}

func TestDiscoveryDoesNotInferRemainingVisibilityFromOneSearch(t *testing.T) {
	search := newSearch(t, catalog())
	for _, name := range search.DeferredToolNames() {
		arguments, err := json.Marshal(map[string]string{"query": "select:" + name})
		if err != nil {
			t.Fatal(err)
		}
		invocation, result := discover(t, search, string(arguments))
		advertised, err := search.AdvertiseDeferred(t.Context(), invocation, result)
		if err != nil || len(advertised) != 1 || advertised[0] != name {
			t.Fatalf("selected %q: advertisement = %v, %v", name, advertised, err)
		}
	}
	output := call(t, search, "select:github_open_pr")
	if !strings.Contains(output, "github_open_pr") {
		t.Fatalf("selected Tool missing from result: %s", output)
	}
	if strings.Contains(strings.ToLower(output), "unloaded") || strings.Contains(strings.ToLower(search.Definition().Description), "not loaded:") {
		t.Fatalf("discovery claimed catalog entries remain hidden after every Tool was advertised:\n%s\n%s", search.Definition().Description, output)
	}
}

func TestAdvertisementRequiresTheStructuredExecutedResult(t *testing.T) {
	search := newSearch(t, catalog())
	invocation, _ := discover(t, search, `{"query":"create issue"}`)
	if _, err := search.AdvertiseDeferred(t.Context(), invocation, toolcontract.Result{Output: toolcontract.NewTextOutput("linear_create_issue")}); err == nil {
		t.Fatal("advertisement inferred matches from display text")
	}
}

func TestUnmatchedDiscoveryAdvertisesNothing(t *testing.T) {
	search := newSearch(t, catalog())
	invocation, result := discover(t, search, `{"query":"select:missing"}`)
	advertised, err := search.AdvertiseDeferred(t.Context(), invocation, result)
	if err != nil || len(advertised) != 0 {
		t.Fatalf("unmatched advertisement = %v, %v", advertised, err)
	}
}

func (m mcpTool) MCPSource() (mcp.CapturedSource, bool) {
	_, remote := m.MCPToolIdentity()
	return mcp.CapturedSource{Config: m.SourceConfig(), RemoteName: remote}, true
}
