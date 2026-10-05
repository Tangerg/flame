package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	executionadapter "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

// P-O04: activating a plugin mid-Session changes the deferred catalog that ends
// each later request, not the tool declarations that open the provider
// prompt-cache prefix.
func TestPluginAcceptanceDiscoveryMidSessionActivationKeepsPrefix(t *testing.T) {
	var remoteCalls atomic.Int32
	remote := sdk.NewServer(&sdk.Implementation{Name: "review", Version: "1"}, nil)
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "record", Description: "Record a review"}, func(_ context.Context, input struct {
		Value string `json:"value"`
	}) (string, error) {
		remoteCalls.Add(1)
		return "recorded " + input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = scopemcp.Register(remote, executable); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(server.Close)

	cfg := runtimeConfigWithRequiredDeps(t)
	cfg.ApprovalMode = approval.ModeYolo
	t.Cleanup(func() {
		_ = filepath.WalkDir(filepath.Join(cfg.Stores.DataDirectory, "plugins"), func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})

	var (
		mu        sync.Mutex
		requests  [][]*chat.Request
		modelName atomic.Value
		phase     atomic.Int32
	)
	modelName.Store("")
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		target := modelName.Load().(string)
		resulted := func(name string) bool {
			for _, message := range request.Messages {
				for _, part := range message.Parts {
					if part.ToolResult != nil && part.ToolResult.Name == name {
						return true
					}
				}
			}
			return false
		}
		mu.Lock()
		for len(requests) < int(phase.Load()) {
			requests = append(requests, nil)
		}
		requests[phase.Load()-1] = append(requests[phase.Load()-1], request.Clone())
		mu.Unlock()
		if phase.Load() == 1 || target == "" || resulted(target) {
			return completedTextResponse("done"), nil
		}
		call := chat.ToolCall{ID: "discover", Name: "search_tools", Arguments: `{"query":"select:` + target + `"}`}
		if slices.ContainsFunc(request.Tools, func(definition chat.ToolDefinition) bool { return definition.Name == target }) {
			call = chat.ToolCall{ID: "record_once", Name: target, Arguments: `{"value":"ok"}`}
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ChatResolver = testChatResolver(client)

	host, api := buildProtocolRuntime(t, cfg, cfg.DefaultWorkspacePath)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.Stores.Idempotency, IdempotencyNamespace: cfg.Stores.IdempotencyNamespace.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		endpoint.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := endpoint.AwaitShutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}}
	must := func(method delivery.Name, input any, key string) any {
		t.Helper()
		attempt := options
		attempt.IdempotencyKey = key
		result := endpoint.Invoke(t.Context(), method, input, attempt)
		if result.Failure != nil {
			t.Fatalf("%s: %v (%v)", method, result.Failure, errors.Unwrap(result.Failure))
		}
		return result.Value
	}
	ctx := protocolLifecycleContext(t.Context())
	session := must(delivery.SessionsCreate, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}}, "session").(*protocol.Session)
	run := func(label string) {
		t.Helper()
		phase.Add(1)
		started, events, err := api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: label}}})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(events), label)
		finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
		if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("%s outcome = %+v, %v", label, finished, err)
		}
	}
	run("before activation")

	source := t.TempDir()
	files := map[string]string{
		"plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"review-package","extensions":{"io.github.tangerg.flame":{"apiVersion":1,"requests":[{"capability":"tools.invoke","targets":["reviews/record"]}]}}}`,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":{"type":"streamable-http","url":"` + server.URL + `"}}}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	installed := must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	must(delivery.PluginsApprove, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, "approve")
	must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable")
	reviews := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	deadline := time.Now().Add(5 * time.Second)
	for modelName.Load().(string) == "" {
		tools := must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: &reviews}, "").(*protocol.Page[protocol.MCPTool])
		if len(tools.Data) == 1 {
			modelName.Store(tools.Data[0].ModelName)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("installation MCP did not connect")
		}
		time.Sleep(10 * time.Millisecond)
	}
	target := modelName.Load().(string)

	run("after activation")

	if remoteCalls.Load() != 1 {
		t.Fatalf("installation tool executions = %d, want 1 through deferred discovery", remoteCalls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || len(requests[0]) == 0 || len(requests[1]) < 2 {
		t.Fatalf("recorded requests per Run = %d, want one Run before and a discovery Run after activation", len(requests))
	}
	before, after := requests[0][0], requests[1][0]
	if slices.ContainsFunc(after.Tools, func(definition chat.ToolDefinition) bool { return definition.Name == target }) {
		t.Fatalf("activation added %s to the base declarations instead of deferring it", target)
	}
	poDiscoveryCompare(t, before, after, target)
	for _, request := range requests[1] {
		if _, found := poDeferredCatalog(t, request); !found {
			t.Fatal("a later model request of the discovery Run lost the deferred catalog")
		}
	}
	items, err := api.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(items.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), poDeferredCatalogMarker) {
		t.Fatal("the deferred catalog was persisted into the Session transcript")
	}
}

// poDeferredCatalogMarker opens the Runtime's per-request deferred catalog.
var poDeferredCatalogMarker = executionadapter.RuntimeContextOpening(executionadapter.RuntimeContextDeferredTools)

// deferredCatalogOf returns the deferred catalog ending request, or "" when the
// request has none. The catalog is a per-request projection: it must be the
// final message, appear exactly once, and so never accumulate in the
// conversation a later call or Run replays.
func deferredCatalogOf(request *chat.Request) (string, error) {
	count := 0
	for _, message := range request.Messages {
		if strings.Contains(message.Text(), poDeferredCatalogMarker) {
			count++
		}
	}
	if count == 0 {
		return "", nil
	}
	last := request.Messages[len(request.Messages)-1]
	if count != 1 || last.Role != chat.RoleUser || !strings.HasPrefix(last.Text(), poDeferredCatalogMarker) {
		return "", fmt.Errorf("deferred catalog messages = %d, final role %s; want exactly one trailing User message", count, last.Role)
	}
	return last.Text(), nil
}

// withoutDeferredCatalog returns the conversation a request carries before its
// trailing per-request catalog.
func withoutDeferredCatalog(request *chat.Request) []chat.Message {
	if catalog, err := deferredCatalogOf(request); err == nil && catalog != "" {
		return request.Messages[:len(request.Messages)-1]
	}
	return request.Messages
}

func poDeferredCatalog(t *testing.T, request *chat.Request) (string, bool) {
	t.Helper()
	catalog, err := deferredCatalogOf(request)
	if err != nil {
		t.Fatal(err)
	}
	return catalog, catalog != ""
}

// poSameDeclarations requires the complete tool declaration block, which opens
// the provider prompt-cache prefix, to be byte-identical.
func poSameDeclarations(t *testing.T, before, after *chat.Request) {
	t.Helper()
	encode := func(definition chat.ToolDefinition) string {
		encoded, err := json.Marshal(definition)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	for index := range max(len(before.Tools), len(after.Tools)) {
		if index >= len(before.Tools) || index >= len(after.Tools) {
			t.Fatalf("tool declaration count changed: %d -> %d", len(before.Tools), len(after.Tools))
		}
		if encode(before.Tools[index]) != encode(after.Tools[index]) {
			t.Fatalf("tool declaration %d changed:\nbefore %s\nafter  %s", index, encode(before.Tools[index]), encode(after.Tools[index]))
		}
	}
}

// poDiscoveryCompare asserts the prefix a provider cache can reuse: the entire
// declaration block and the system prompt stay byte-identical, while the
// trailing catalog message reflects the activation.
func poDiscoveryCompare(t *testing.T, before, after *chat.Request, target string) {
	t.Helper()
	poSameDeclarations(t, before, after)
	beforeCatalog, found := poDeferredCatalog(t, before)
	if !found {
		t.Fatal("the Run before activation has no deferred catalog")
	}
	afterCatalog, found := poDeferredCatalog(t, after)
	if !found {
		t.Fatal("the Run after activation has no deferred catalog")
	}
	if strings.Contains(beforeCatalog, target) || !strings.Contains(afterCatalog, target) {
		t.Fatalf("deferred catalog does not reflect activation:\n%s\n---\n%s", beforeCatalog, afterCatalog)
	}
	system := func(request *chat.Request) string {
		var b strings.Builder
		for _, message := range request.Messages {
			if message.Role == chat.RoleSystem {
				b.WriteString(message.Text())
			}
		}
		return b.String()
	}
	if system(before) == "" || system(before) != system(after) {
		t.Fatal("system prompt changed after activation")
	}
}

// An MCP reconnect that changes the server's tool list reaches later Runs only
// through the deferred catalog; the declaration block stays byte-identical.
func TestPluginAcceptanceDiscoveryMCPReconnectKeepsDeclarations(t *testing.T) {
	remote := sdk.NewServer(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
	register := func(name string) {
		t.Helper()
		executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: name, Description: "Fixture tool " + name}, func(_ context.Context, input struct {
			Value string `json:"value"`
		}) (string, error) {
			return input.Value, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := scopemcp.Register(remote, executable); err != nil {
			t.Fatal(err)
		}
	}
	register("record")
	server := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(server.Close)

	cfg := poMCPConfig(t)
	var (
		mu     sync.Mutex
		firsts []*chat.Request
		phase  atomic.Int32
	)
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) != 0 {
			mu.Lock()
			if len(firsts) < int(phase.Load()) {
				firsts = append(firsts, request.Clone())
			}
			mu.Unlock()
		}
		return completedTextResponse("done"), nil
	})}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ChatResolver = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	runtime.must(delivery.MCPServersCreate, protocol.MCPServerCandidate{
		Name: "reviews", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: server.URL},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-reviews")
	reviews := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: "reviews"}
	runtime.poMCPTools(&reviews, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })

	ctx := protocolLifecycleContext(t.Context())
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	run := func(label string) {
		t.Helper()
		phase.Add(1)
		_, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: label}}})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(events), label)
	}
	run("before reconnect")

	register("summarize")
	runtime.must(delivery.MCPServersReconnect, protocol.MCPServerRequest{Server: reviews}, "reconnect")
	var added string
	for _, tool := range runtime.poMCPTools(&reviews, func(tools []protocol.MCPTool) bool { return len(tools) == 2 }) {
		if tool.Name == "summarize" {
			added = tool.ModelName
		}
	}
	if added == "" {
		t.Fatal("reconnect did not surface the added tool")
	}
	run("after reconnect")

	mu.Lock()
	defer mu.Unlock()
	if len(firsts) != 2 {
		t.Fatalf("recorded first requests = %d, want 2", len(firsts))
	}
	before, after := firsts[0], firsts[1]
	poSameDeclarations(t, before, after)
	beforeCatalog, _ := poDeferredCatalog(t, before)
	afterCatalog, found := poDeferredCatalog(t, after)
	if !found || strings.Contains(beforeCatalog, added) || !strings.Contains(afterCatalog, added) {
		t.Fatalf("deferred catalog does not reflect the reconnect:\n%s\n---\n%s", beforeCatalog, afterCatalog)
	}
}

// Restoring a waiting Run rebuilds its Discovery from the same frozen deferred
// set, so the catalog after restart is the same projection, not a stored copy.
func TestDeferredCatalogSurvivesWaitingRestoreUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	var (
		mu       sync.Mutex
		catalogs []string
	)
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) == 0 {
			return completedTextResponse("title"), nil
		}
		catalog, err := deferredCatalogOf(request)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		catalogs = append(catalogs, catalog)
		calls := len(catalogs)
		mu.Unlock()
		if calls > 1 {
			return completedTextResponse("done"), nil
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
			ID: "shell_approval", Name: "shell", Arguments: `{"command":"printf ok > ok.txt","description":"Write ok"}`,
		}))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}
	ctx := delivery.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion:    protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}},
	})
	first, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Run the tool after approval."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "approval before restart")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: started.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("restored approval = %+v, %v", pending, err)
	}
	_, events, err = api.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: started.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: pending.Data[0].Interrupts[0].ItemID,
			Response: protocol.InterruptResponseValue{
				Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "approval after restart")
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("restored Run = %+v, %v", finished, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(catalogs) != 2 || catalogs[0] == "" || catalogs[0] != catalogs[1] {
		t.Fatalf("deferred catalogs before and after restore = %q", catalogs)
	}
	items, err := api.ListItems(ctx, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(items.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), poDeferredCatalogMarker) {
		t.Fatal("the deferred catalog was persisted into the Session transcript")
	}
}
