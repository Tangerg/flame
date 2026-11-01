package bootstrap

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/qimu/portable"
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

func poMCPOpen(t *testing.T, cfg assemblyConfig) *poMCPRuntime {
	t.Helper()
	host, api := buildPluginProtocolRuntime(t, cfg)
	runtime := &poMCPRuntime{t: t, host: host, api: api, endpoint: host.Endpoint()}
	t.Cleanup(runtime.close)
	return runtime
}

func buildPluginProtocolRuntime(t *testing.T, cfg assemblyConfig) (*Instance, *delivery.Handler) {
	t.Helper()
	host := assembleTestRuntime(t, cfg)
	api, err := pluginProtocolHandler(host, cfg)
	if err != nil {
		_ = host.Close()
		t.Fatalf("build plugin delivery handler: %v", err)
	}
	startTestRuntime(t, host, api, cfg.stores)
	return host, api
}

func pluginProtocolHandler(host *Instance, cfg assemblyConfig) (*delivery.Handler, error) {
	// Plugin callbacks retain Core identity across restart. Discovery and the
	// Endpoint must use the store's namespace, including in custom loss fixtures.
	return host.application.newDeliveryHandler(protocol.ServerInfo{
		Name: "plugin-acceptance-test", Version: "0.0.0-test", InstanceID: testsupport.RuntimeInstanceID,
		DefaultWorkspace: protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}, Home: cfg.userHome,
	}, cfg.stores.IdempotencyNamespace.String())
}

func (r *poMCPRuntime) close() {
	if r.closed {
		return
	}
	r.closed = true
	if err := r.host.Close(); err != nil {
		r.t.Error(err)
	}
}

func (r *poMCPRuntime) must(method delivery.Name, input any, key string) any {
	r.t.Helper()
	options := delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}, IdempotencyKey: key}
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
			for _, value := range servers.Data {
				status, err := json.Marshal(value.Status)
				r.t.Logf("MCP discovery deadline: server=%s status=%s encoding_error=%v", value.ID.Name, status, err)
			}
			// A connecting projection omits the blocked handshake phase. Capture its
			// live owner before test cleanup cancels and joins that evidence.
			stack := make([]byte, 1<<20)
			r.t.Logf("MCP discovery deadline goroutines:\n%s", stack[:goruntime.Stack(stack, true)])
			r.t.Fatalf("MCP tools never settled: %+v; servers: %+v", tools.Data, servers.Data)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func poMCPRemote(t *testing.T, names ...string) string {
	t.Helper()
	remote := sdk.NewServer(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
	publishTestBackend(t, remote)
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

func poMCPConfig(t *testing.T) assemblyConfig {
	t.Helper()
	cfg := runtimeConfigWithRequiredDeps(t)
	t.Cleanup(func() {
		_ = filepath.WalkDir(filepath.Join(cfg.stores.DataDirectory, "plugins"), func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	return cfg
}

func installExternalConnector(r *poMCPRuntime) *protocol.PluginInstallation {
	r.t.Helper()
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "mcp")}, "install-external-connector").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-external-connector")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: mcpconnector.Reference, InstallationID: installed.ID, Contribution: mcpconnector.Export}, "select-external-connector")
	return installed
}

func poMCPActivate(r *poMCPRuntime, source string) *protocol.PluginInstallation {
	r.t.Helper()
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install").(*protocol.PluginInstallation)
	installed = r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable").(*protocol.PluginInstallation)
	for _, contribution := range installed.Selected.Contributions {
		contract, err := portable.ParseCapability([]byte(contribution.Contract))
		if err != nil {
			r.t.Fatal(err)
		}
		if contract.Descriptor().Composition != portable.Selection {
			continue
		}
		role := contract.Descriptor().Reference
		r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: role, InstallationID: installed.ID, Contribution: contribution.ID}, "select-"+role)
	}
	return installed
}

const poMCPManifest = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"review-package","version":"1.0.0"}`

func TestPluginAcceptanceMCPInvalidServerKeepsSiblingsAcrossRestart(t *testing.T) {
	url := poMCPRemote(t, "record")
	cfg := poMCPConfig(t)
	source := poMCPPackage(t, map[string]string{
		"plugin.json": poMCPManifest,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":` + testMCPServerJSON(t, url) + `,"broken":{"type":"carrier-pigeon"}}}`,
	})
	authorPluginPackage(t, source)
	first := poMCPOpen(t, cfg)
	enabled := poMCPActivate(first, source)
	broken := protocol.PluginDiagnostic{Component: protocol.PluginComponent{Type: protocol.PluginComponentMCPServer, Name: new("broken")}, Code: protocol.PluginDiagnosticInvalidDeclaration}
	if !slices.ContainsFunc(enabled.Selected.Diagnostics, func(d protocol.PluginDiagnostic) bool { return reflect.DeepEqual(d, broken) }) {
		t.Fatalf("invalid server is not diagnosed by identity: %+v", enabled.Selected.Diagnostics)
	}
	if len(enabled.Selected.Servers) != 1 || enabled.Selected.Servers[0].Name != "reviews" {
		t.Fatalf("invalid server was admitted or withdrew its sibling: %+v", enabled.Selected.Servers)
	}
	if enabled.Enabled != true || enabled.Presentation != protocol.PluginPresentationAdmitted || enabled.Realization.Type != protocol.PluginRealizationAvailable {
		t.Fatalf("installation with one invalid server did not activate: %+v", enabled)
	}
	sibling := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: enabled.ID}, Name: "reviews"}
	hasRecord := func(tools []protocol.MCPTool) bool {
		return slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "record" })
	}
	first.poMCPTools(&sibling, hasRecord)
	restarted := reopenPluginRuntime(t, first, &cfg)
	listed := restarted.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	if len(listed.Data) != 1 {
		t.Fatalf("restart lost the installation: %+v", listed.Data)
	}
	cold := listed.Data[0]
	if cold.ID != enabled.ID || cold.Enabled != true || cold.Presentation != protocol.PluginPresentationAdmitted ||
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

func TestPluginAcceptanceMCPSameNameAcrossOriginsPublishesDistinctToolAliases(t *testing.T) {
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
	cfg.modelProviders = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	installExternalConnector(runtime)
	runtime.must(delivery.MCPServersCreate, protocol.MCPServerCandidate{
		Name: "reviews", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: userURL},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-user-reviews")
	source := poMCPPackage(t, map[string]string{
		"plugin.json": poMCPManifest,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":` + testMCPServerJSON(t, installationURL) + `}}`,
	})
	authorPluginPackage(t, source)
	installed := poMCPActivate(runtime, source)
	user := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: "reviews"}
	owned := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "reviews"}
	userRef := protocol.ToolRef{Type: protocol.ToolRefMCP, Name: "record", Server: &user}
	ownedRef := protocol.ToolRef{Type: protocol.ToolRefMCP, Name: "record", Server: &owned}
	tools := runtime.poMCPTools(nil, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
	var externalAlias, installedAlias, unrelated string
	for _, tool := range tools {
		switch {
		case tool.Server == user && tool.Name == "record":
			externalAlias = tool.ModelName
			if !slices.ContainsFunc(tool.NameConflicts, func(ref protocol.ToolRef) bool { return reflect.DeepEqual(ref, ownedRef) }) {
				t.Fatalf("user tool diagnostic does not name the installation identity: %+v", tool.NameConflicts)
			}
		case tool.Server == owned && tool.Name == "record":
			installedAlias = tool.ModelName
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
	if externalAlias == "" || installedAlias == "" || unrelated == "" || externalAlias == installedAlias {
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
	names := strings.Fields(strings.ReplaceAll(catalog, ",", " "))
	for _, exact := range []string{externalAlias, installedAlias, unrelated} {
		if !slices.Contains(names, exact) {
			t.Fatalf("model catalog omits %q: %s", exact, catalog)
		}
	}
	if slices.Contains(names, "reviews_record") {
		t.Fatalf("ambiguous bare name remained callable: %s", catalog)
	}

}

func TestMCPInstallationAndExternalServerWithdrawIndependently(t *testing.T) {
	cfg := poMCPConfig(t)
	const proof = "installed-tool-still-executes"
	var target atomic.Value
	target.Store("")
	var executed atomic.Bool
	model, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(_ context.Context, req *chat.Request) (*chat.Response, error) {
		name := target.Load().(string)
		if name == "" || len(req.Tools) == 0 {
			return completedTextResponse("ready"), nil
		}
		for _, msg := range req.Messages {
			for _, part := range msg.Parts {
				if part.ToolResult != nil && part.ToolResult.Name == name {
					reply, err := json.Marshal(part.ToolResult.Output)
					if err != nil {
						return nil, err
					}
					executed.Store(!part.ToolResult.IsError && bytes.Contains(reply, []byte(proof)))
					return completedTextResponse("finished"), nil
				}
			}
		}
		call := chat.ToolCall{ID: "find-independent-tool", Name: "search_tools", Arguments: `{"query":"select:` + name + `"}`}
		if slices.ContainsFunc(req.Tools, func(d chat.ToolDefinition) bool { return d.Name == name }) {
			call = chat.ToolCall{ID: "invoke-independent-tool", Name: name, Arguments: `{"value":"` + proof + `"}`}
		}
		msg := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{Message: &msg, FinishReason: chat.FinishReasonToolCalls}, nil)
	})}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(model)
	cfg.approvalMode = approval.ModeYolo

	current := poMCPOpen(t, cfg)
	const name = "reviews"
	external := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: name}
	connector := installExternalConnector(current)
	current.must(delivery.MCPServersCreate, protocol.MCPServerCandidate{
		Name: name, Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: poMCPRemote(t, "record")},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-external")
	source := poMCPPackage(t, map[string]string{
		"plugin.json": poMCPManifest,
		"mcp.json":    `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"reviews":` + testMCPServerJSON(t, poMCPRemote(t, "record")) + `}}`,
	})
	authorPluginPackage(t, source)
	installation := poMCPActivate(current, source)
	backend := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installation.ID}, Name: name}
	expectOwners := func(expected ...protocol.MCPServerID) {
		t.Helper()
		current.poMCPTools(nil, func(tools []protocol.MCPTool) bool {
			var owners []protocol.MCPServerID
			for _, tool := range tools {
				if tool.Server.Name != name || tool.Name != "record" {
					continue
				}
				if len(expected) == 1 && len(tool.NameConflicts) > 0 {
					return false
				}
				owners = append(owners, tool.Server)
			}
			if len(owners) != len(expected) {
				return false
			}
			for _, owner := range expected {
				if !slices.Contains(owners, owner) {
					return false
				}
			}
			return true
		})
	}
	expectOwners(external, backend)
	current.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: connector.ID, Enabled: false}, "withdraw-external-connector")
	expectOwners(backend)
	modelTools := current.poMCPTools(&backend, func(tools []protocol.MCPTool) bool { return len(tools) == 1 })
	target.Store(modelTools[0].ModelName)
	ctx := protocolLifecycleContext(t.Context())
	session, err := current.api.CreateSession(ctx, protocol.CreateSessionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_, events, err := current.api.StartRun(ctx, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Execute the installed tool."}}})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "tool call with external connector disabled")
	if !executed.Load() {
		t.Fatal("installed tool did not return its original result")
	}
	current = reopenPluginRuntime(t, current, &cfg)
	expectOwners(backend)
	current.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: connector.ID, Enabled: true}, "restore-external-connector")
	expectOwners(backend, external)
	current.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installation.ID, Enabled: false}, "disable-backend")
	expectOwners(external)
	current.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installation.ID, Enabled: true}, "enable-backend")
	expectOwners(backend, external)
	current.must(delivery.MCPServersDelete, protocol.MCPServerRequest{Server: external}, "remove-external")
	expectOwners(backend)
	current = reopenPluginRuntime(t, current, &cfg)
	expectOwners(backend)
}

func TestExternalConnectorKeepsLaunchSecretsOutOfPluginAndRestoresAuthorizationReceipt(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := installExternalConnector(r)
	const secret = "external-mcp-test-private-header"
	seen := make(chan struct{}, 1)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.Header.Get("X-Fixture-Key") == secret {
			select {
			case seen <- struct{}{}:
			default:
			}
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer remote.Close()
	server := r.must(delivery.MCPServersCreate, protocol.MCPServerCandidate{
		Name: "signed", Enabled: true,
		Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: remote.URL, Headers: &protocol.MCPHeadersChange{Type: protocol.MCPSecretSet, Value: map[string]string{"X-Fixture-Key": secret}}},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}, "create-protected-remote").(*protocol.MCPServer)
	select {
	case <-seen:
	case <-time.After(10 * time.Second):
		t.Fatal("approved transport did not receive its header")
	}
	attempt, err := r.api.CreateMCPAuthorizationAttempt(t.Context(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for attempt.Status.Type == protocol.MCPAuthorizationAttemptPending {
		if time.Now().After(deadline) {
			t.Fatal("authorization never settled")
		}
		attempt, err = r.api.GetMCPAuthorizationAttempt(t.Context(), attempt.ID)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if attempt.Status.Type != protocol.MCPAuthorizationAttemptFailed {
		t.Fatalf("unavailable OAuth endpoint settled as %s", attempt.Status.Type)
	}
	r = reopenPluginRuntime(t, r, &cfg)
	restored, err := r.api.GetMCPAuthorizationAttempt(t.Context(), attempt.ID)
	if err != nil || !reflect.DeepEqual(restored, attempt) {
		t.Fatalf("original authorization receipt lost: %+v %v", restored, err)
	}
	data := filepath.Join(cfg.stores.DataDirectory, "plugins", "data", installed.ID)
	var files int
	err = filepath.WalkDir(data, func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files++
		if bytes.Contains(body, []byte(secret)) || bytes.Contains(body, []byte(remote.URL)) {
			return errors.New("plugin received private transport material")
		}
		return nil
	})
	if err != nil || files == 0 {
		t.Fatalf("connector data separation: files=%d, error=%v", files, err)
	}
}
