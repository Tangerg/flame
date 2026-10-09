package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

// poMCPRuntime is one Runtime instance reached through the delivery endpoint,
// the same entry both the Go binding and the Runtime Protocol use.
type poMCPRuntime struct {
	t        *testing.T
	host     *Instance
	api      *delivery.Handler
	endpoint *delivery.Endpoint
	closed   bool
}

func poMCPOpen(t *testing.T, cfg Config) *poMCPRuntime {
	t.Helper()
	host, api := buildProtocolRuntime(t, cfg, cfg.DefaultWorkspacePath)
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.Stores.Idempotency, IdempotencyNamespace: cfg.Stores.IdempotencyNamespace.String()})
	if err != nil {
		_ = host.Close()
		t.Fatal(err)
	}
	runtime := &poMCPRuntime{t: t, host: host, api: api, endpoint: endpoint}
	t.Cleanup(runtime.close)
	return runtime
}

func (r *poMCPRuntime) close() {
	if r.closed {
		return
	}
	r.closed = true
	r.endpoint.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.endpoint.AwaitShutdown(ctx); err != nil {
		r.t.Error(err)
	}
	if err := r.host.Close(); err != nil {
		r.t.Error(err)
	}
}

func (r *poMCPRuntime) must(method delivery.Name, input any, key string) any {
	r.t.Helper()
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}, IdempotencyKey: key}
	result := r.endpoint.Invoke(r.t.Context(), method, input, options)
	if result.Failure != nil {
		r.t.Fatalf("%s: %v (%v)", method, result.Failure, errors.Unwrap(result.Failure))
	}
	return result.Value
}

// poMCPTools waits until the listing satisfies ready; connections settle in the
// background after a command commits.
func (r *poMCPRuntime) poMCPTools(server *protocol.MCPServerID, ready func([]protocol.MCPTool) bool) []protocol.MCPTool {
	r.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		tools := r.must(delivery.MCPToolsList, protocol.MCPListToolsRequest{Server: server}, "").(*protocol.Page[protocol.MCPTool])
		if ready(tools.Data) {
			return tools.Data
		}
		if time.Now().After(deadline) {
			servers := r.must(delivery.MCPServersList, struct{}{}, "").(*protocol.Page[protocol.MCPServer])
			r.t.Fatalf("MCP tools never settled: %+v; servers: %+v", tools.Data, servers.Data)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func poMCPRemote(t *testing.T, names ...string) string {
	t.Helper()
	remote := sdk.NewServer(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
	for _, name := range names {
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
	server := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(server.Close)
	return server.URL
}

func poMCPPackage(t *testing.T, files map[string]string) string {
	t.Helper()
	source := t.TempDir()
	for name, body := range files {
		file := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return source
}

func poMCPConfig(t *testing.T) Config {
	t.Helper()
	cfg := runtimeConfigWithRequiredDeps(t)
	t.Cleanup(func() {
		_ = filepath.WalkDir(filepath.Join(cfg.Stores.DataDirectory, "plugins"), func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	return cfg
}

func poMCPActivate(r *poMCPRuntime, source string) *protocol.PluginInstallation {
	r.t.Helper()
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	r.must(delivery.PluginsApprove, protocol.PluginReleaseRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest}, "approve")
	return r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable").(*protocol.PluginInstallation)
}

const poMCPManifest = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"review-package","extensions":{"io.github.tangerg.flame":{"apiVersion":1,"requests":[{"capability":"tools.invoke","targets":["reviews/record","reviews/summarize"]}]}}}`

func TestPluginAcceptanceMCPInvalidServerKeepsSiblingsAcrossRestart(t *testing.T) {
	url := poMCPRemote(t, "record")
	cfg := poMCPConfig(t)
	source := poMCPPackage(t, map[string]string{
		"plugin.json": poMCPManifest,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":{"type":"streamable-http","url":"` + url + `"},"broken":{"type":"carrier-pigeon"}}}`,
	})
	first := poMCPOpen(t, cfg)
	enabled := poMCPActivate(first, source)
	broken := protocol.PluginDiagnostic{Component: protocol.PluginComponent{Type: protocol.PluginComponentMCPServer, Name: new("broken")}, Code: protocol.PluginDiagnosticInvalidDeclaration}
	if !slices.ContainsFunc(enabled.Selected.Diagnostics, func(d protocol.PluginDiagnostic) bool { return reflect.DeepEqual(d, broken) }) {
		t.Fatalf("invalid server is not diagnosed by identity: %+v", enabled.Selected.Diagnostics)
	}
	if len(enabled.Selected.Servers) != 1 || enabled.Selected.Servers[0].Name != "reviews" {
		t.Fatalf("invalid server was admitted or withdrew its sibling: %+v", enabled.Selected.Servers)
	}
	if enabled.State != protocol.PluginInstallationEnabled || enabled.Presentation != protocol.PluginPresentationAdmitted || enabled.Realization.Type != protocol.PluginRealizationAvailable {
		t.Fatalf("installation with one invalid server did not activate: %+v", enabled)
	}
	sibling := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: enabled.ID}, Name: "reviews"}
	hasRecord := func(tools []protocol.MCPTool) bool {
		return slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "record" })
	}
	first.poMCPTools(&sibling, hasRecord)
	first.close()

	restarted := poMCPOpen(t, cfg)
	listed := restarted.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	if len(listed.Data) != 1 {
		t.Fatalf("restart lost the installation: %+v", listed.Data)
	}
	cold := listed.Data[0]
	if cold.ID != enabled.ID || cold.State != protocol.PluginInstallationEnabled || cold.Presentation != protocol.PluginPresentationAdmitted ||
		cold.Selected.Digest != enabled.Selected.Digest || !reflect.DeepEqual(cold.Selected.Diagnostics, enabled.Selected.Diagnostics) {
		t.Fatalf("restart changed the installation: %+v, want %+v", cold, enabled)
	}
	restarted.poMCPTools(&sibling, hasRecord)
	servers := restarted.must(delivery.MCPServersList, struct{}{}, "").(*protocol.Page[protocol.MCPServer])
	for _, server := range servers.Data {
		if server.ID.Name == "broken" {
			t.Fatalf("invalid declaration became a server after restart: %+v", server)
		}
	}
}

func TestPluginAcceptanceMCPSameNameAcrossOriginsExcludesBoth(t *testing.T) {
	userURL := poMCPRemote(t, "record")
	installationURL := poMCPRemote(t, "record", "summarize")
	cfg := poMCPConfig(t)
	var mu sync.Mutex
	var catalog string
	model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(request.Tools) != 0 {
			catalog, _ = poDeferredCatalog(t, request)
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
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: userURL},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-user-reviews")
	installed := poMCPActivate(runtime, poMCPPackage(t, map[string]string{
		"plugin.json": poMCPManifest,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":{"type":"streamable-http","url":"` + installationURL + `"}}}`,
	}))
	user := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: "reviews"}
	owned := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	userRef := protocol.ToolRef{Type: protocol.ToolRefMCP, Name: "record", Server: &user}
	ownedRef := protocol.ToolRef{Type: protocol.ToolRefMCP, Name: "record", Server: &owned}
	tools := runtime.poMCPTools(nil, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	var modelName, unrelated string
	for _, tool := range tools {
		switch {
		case tool.Server == user && tool.Name == "record":
			modelName = tool.ModelName
			if !slices.ContainsFunc(tool.NameConflicts, func(ref protocol.ToolRef) bool { return reflect.DeepEqual(ref, ownedRef) }) {
				t.Fatalf("user tool diagnostic does not name the installation identity: %+v", tool.NameConflicts)
			}
		case tool.Server == owned && tool.Name == "record":
			if !slices.ContainsFunc(tool.NameConflicts, func(ref protocol.ToolRef) bool { return reflect.DeepEqual(ref, userRef) }) {
				t.Fatalf("installation tool diagnostic does not name the user identity: %+v", tool.NameConflicts)
			}
		case tool.Server == owned && tool.Name == "summarize":
			unrelated = tool.ModelName
			if len(tool.NameConflicts) != 0 {
				t.Fatalf("unrelated tool reports a conflict: %+v", tool.NameConflicts)
			}
		default:
			t.Fatalf("unexpected tool %+v", tool)
		}
	}
	if modelName == "" || unrelated == "" {
		t.Fatalf("listing lacks a projected tool: %+v", tools)
	}

	ctx := protocolLifecycleContext(t.Context())
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "List the reachable tools."}}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "conflict manifest run")
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(catalog, unrelated) {
		t.Fatalf("deferred catalog omits the unrelated installation tool %q:\n%s", unrelated, catalog)
	}
	if strings.Contains(catalog, modelName) {
		t.Fatalf("conflicting model name %q remained reachable:\n%s", modelName, catalog)
	}
}
