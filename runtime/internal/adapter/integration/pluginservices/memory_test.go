package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	hostsqlite "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	app "github.com/Tangerg/flame/runtime/internal/plugins/memory/application"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	memorysqlite "github.com/Tangerg/flame/runtime/internal/plugins/memory/sqlite"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/transport"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

type capturedConversation []chat.Message

func (c capturedConversation) Read(context.Context, string) ([]chat.Message, error) { return c, nil }

func TestMemoryMaintenancePersistsBusinessInputInBothOwnersAndSearchResolutionFailureIsExplicit(t *testing.T) {
	name := testsupport.ServerName("memory")
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.memory", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "echo"}}, Contributions: []plugin.Contribution{{ID: "memory-provider", Server: name, Export: "memory", Contract: contracts.Capability.Identity()}}})
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), "test source", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	source, err := installation.ServerSource(release, name)
	if err != nil {
		t.Fatal(err)
	}
	store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	private, err := memorysqlite.Open(t.Context(), filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	remote := sdk.NewServer(&sdk.Implementation{Name: "memory"}, nil)
	roots, err := workspaceapp.NewScope("", "", workspace.Resolver{})
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.RegisterServer(remote, private, memorysqlite.NewAgentMemoryStore(private), roots); err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(endpoint.Close)
	server := mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: endpoint.URL}
	callbacks := pluginsdk.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	pool, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, toolSourceRegistry{server}, callbacks, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	attempt, err := pool.BeginConnect(t.Context(), server.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer attempt.Close()
	if err := attempt.Run(); err != nil {
		t.Fatal(err)
	}
	host, err := hostsqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	selections := hostsqlite.NewProviderSelectionStore(host)
	choice, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: contracts.Capability.Descriptor().Reference, Installation: installation.ID(), Contribution: "memory-provider"})
	if err != nil {
		t.Fatal(err)
	}
	if err := selections.Save(t.Context(), choice); err != nil {
		t.Fatal(err)
	}
	routing, err := plugins.NewCommands(hostsqlite.NewPluginCommandStore(host), plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	calls, err := NewCommandCalls(store, selections, pool, routing)
	if err != nil {
		t.Fatal(err)
	}
	var modelCalls atomic.Int32
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		modelCalls.Add(1)
		message := chat.NewAssistantMessage(chat.NewTextPart("- callback-only model fact"))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	})
	history := capturedConversation{chat.NewUserMessage(chat.NewTextPart("captured conversation")), chat.NewAssistantMessage(chat.NewTextPart("retained response")), chat.NewUserMessage(chat.NewTextPart("continued")), chat.NewAssistantMessage(chat.NewTextPart("complete"))}
	resolutionFailure := errors.New("configured embedding role is unavailable")
	owner, err := NewMemory(MemoryConfig{Installations: store, Connections: pool, Callbacks: callbacks, Commands: calls, Conversation: history, Utility: models.AuxiliaryResolver(func(context.Context) (chat.Model, error) { return model, nil }), Embeddings: func(context.Context) (app.Embedder, error) { return nil, resolutionFailure }})
	if err != nil {
		t.Fatal(err)
	}
	project, err := (workspace.Resolver{}).ResolveExistingDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.MaintainMemory(t.Context(), execution.RunMaintenanceInput{SessionID: "ses_metadata", WorkspaceCWD: project, DurableContextCompacted: true}); err != nil {
		t.Fatal(err)
	}
	if modelCalls.Load() != 2 {
		t.Fatalf("actual maintenance callbacks=%d", modelCalls.Load())
	}
	var id, hostInput, phase, privateInput string
	if err := host.QueryRowContext(t.Context(), `SELECT id,business_input_json,phase FROM plugin_commands WHERE method=?`, rpc.Maintain).Scan(&id, &hostInput, &phase); err != nil {
		t.Fatal(err)
	}
	if err := private.QueryRowContext(t.Context(), `SELECT business_input_json FROM commands WHERE id=?`, id).Scan(&privateInput); err != nil {
		t.Fatal(err)
	}
	var input rpc.MaintenanceInput
	if err := json.Unmarshal([]byte(hostInput), &input, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if input.SessionID != "ses_metadata" || input.Project != project || !input.Compacted || input.Conversation.MessageCount != len(history) || input.Conversation.Transcript == "" || hostInput != privateInput || phase != string(plugin.CommandObserved) {
		t.Fatalf("original business-only routing/receipt: %+v, equal=%t, phase=%s", input, hostInput == privateInput, phase)
	}
	if _, err := owner.Search(t.Context(), project, "fact", 5); !errors.Is(err, resolutionFailure) {
		t.Fatalf("configured role failure became absence: %v", err)
	}
	owner.embedder = func(context.Context) (app.Embedder, error) { return nil, nil }
	addContext := operation.Bind(t.Context(), "memory.add", "keyword")
	if _, err := owner.Add(addContext, memory.ScopeProject, project, "keyword fact"); err != nil {
		t.Fatal(err)
	}
	stored, found, err := hostsqlite.NewPluginCommandStore(host).Get(t.Context(), operation.Identity(addContext))
	if err != nil || !found {
		t.Fatalf("original add route: found=%v, %v", found, err)
	}
	request := plugins.CallRequest{Installation: installation.ID(), Digest: release.Digest(), Contribution: "memory-provider", Contract: contracts.Capability, Method: rpc.Add, Input: stored.Snapshot().Input, Target: stored.Snapshot().Invocation.Target}
	installed, err := calls.Prepare(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installed.Invoke(addContext); err != nil {
		t.Fatalf("installed entrance did not recover the feature owner's receipt: %v", err)
	}
	if _, err := installed.Invoke(operation.Adopt(t.Context(), operation.Identity(addContext), capability.Model, "parent-tool")); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("changed calling origin reused an original identity: %v", err)
	}
	changed := request
	changed.Contribution = "another-provider"
	other, err := calls.Prepare(t.Context(), changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Invoke(addContext); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("changed installed binding reused an original identity: %v", err)
	}
	items, err := owner.Search(t.Context(), project, "keyword", 5)
	if err != nil || len(items) != 1 || items[0].Content() != "keyword fact" {
		t.Fatalf("absent role lost keyword search: %v, %v", items, err)
	}
	embedder := &preparedEmbedder{}
	owner.embedder = func(context.Context) (app.Embedder, error) { return embedder, nil }
	items, err = owner.Search(t.Context(), project, "keyword", 5)
	if err != nil || len(items) != 1 || embedder.calls.Load() != 2 {
		t.Fatalf("human search callback metadata: items=%v, embeddings=%d, %v", items, embedder.calls.Load(), err)
	}
}
