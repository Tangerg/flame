package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/idempotency"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	memory "github.com/Tangerg/flame/plugins/memory/backend/domain"
	"github.com/Tangerg/flame/plugins/memory/backend/rpc"
	memorysqlite "github.com/Tangerg/flame/plugins/memory/backend/sqlite"
	"github.com/Tangerg/flame/plugins/memory/backend/transport"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

type capturedConversation []chat.Message

func (c capturedConversation) Read(context.Context, string) ([]chat.Message, error) { return c, nil }

type ownedMemoryEmbedder struct {
	preparedEmbedder
	t      *testing.T
	parent string
}

func (e *ownedMemoryEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil || invocation.Origin != capability.Model || invocation.Parent != e.parent || invocation.ID == e.parent {
		e.t.Errorf("Memory embedding detached from its original query: %+v, %v", invocation, err)
	}
	return e.preparedEmbedder.Embed(ctx, texts)
}

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
	pool, err := mcp.Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, toolSourceRegistry{server}, callbacks, nil, nil, nil)
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
	host, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "flame.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	selections := host.ProviderSelections
	choice, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: contracts.Capability.Descriptor().Reference, Installation: installation.ID(), Contribution: "memory-provider"})
	if err != nil {
		t.Fatal(err)
	}
	if err := selections.Save(t.Context(), choice); err != nil {
		t.Fatal(err)
	}
	admission := plugins.NewAdmission()
	routing, err := plugins.NewCommands(host.PluginCommands, admission)
	if err != nil {
		t.Fatal(err)
	}
	var notices []invalidation.Notice
	publication := invalidation.Publish(func(notice invalidation.Notice) { notices = append(notices, notice) })
	calls, err := NewCommandCalls(store, selections, host.PluginCompositions, pool, routing, publication)
	if err != nil {
		t.Fatal(err)
	}
	observers, err := NewCallComposition(calls, callbacks, admission)
	if err != nil {
		t.Fatal(err)
	}
	var modelCalls, releases atomic.Int32
	modelInvocations := make(chan capability.Invocation, 2)
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		invocation, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil {
			return nil, err
		}
		modelInvocations <- invocation
		modelCalls.Add(1)
		message := chat.NewAssistantMessage(chat.NewTextPart("- callback-only model fact"))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	})
	history := capturedConversation{chat.NewUserMessage(chat.NewTextPart("captured conversation")), chat.NewAssistantMessage(chat.NewTextPart("retained response")), chat.NewUserMessage(chat.NewTextPart("continued")), chat.NewAssistantMessage(chat.NewTextPart("complete"))}
	resolutionFailure := errors.New("configured embedding role is unavailable")
	utility := observers.Auxiliary(models.AuxiliaryResolver(func(context.Context, models.AuxiliaryBudget) (models.SelectedChat, func(), error) {
		return auxiliaryFixture(model), func() { releases.Add(1) }, nil
	}))
	owner, err := NewMemory(MemoryConfig{Installations: store, Connections: pool, Callbacks: callbacks, Commands: calls, Conversation: history, Utility: utility, Embeddings: func(context.Context) (models.TextEmbedder, error) { return nil, resolutionFailure }})
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
	if releases.Load() != 1 || len(observers.HeldDependencies()) != 0 {
		t.Fatal("maintenance did not release its single utility capture")
	}
	var id, hostInput, phase, privateInput string
	if err := host.QueryRowContext(t.Context(), `SELECT id,business_input_json,phase FROM plugin_commands WHERE method=?`, rpc.Maintain).Scan(&id, &hostInput, &phase); err != nil {
		t.Fatal(err)
	}
	first, second := <-modelInvocations, <-modelInvocations
	if first.ID == second.ID || first.Parent != id || second.Parent != id || first.Origin != capability.Automatic || second.Origin != capability.Automatic || first.Target.Scope != "call" || second.Target.Scope != "call" {
		t.Fatalf("model calls lost distinct original ancestry: %+v, %+v", first, second)
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
	owner.embedder = func(context.Context) (models.TextEmbedder, error) { return nil, nil }
	if err := callbacks.PublishRPC(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("human memory addition unexpectedly called Runtime RPC: %s", r.URL.Path)
		http.Error(w, "unexpected callback", http.StatusBadRequest)
	})); err != nil {
		t.Fatal(err)
	}
	addContext := operation.Bind(t.Context(), "memory.add", "keyword")
	addition, err := jsoninput.Encode(rpc.AddInput{Scope: memory.ScopeProject, CWD: project, Content: "keyword fact"})
	if err != nil {
		t.Fatal(err)
	}
	publicAdd, err := calls.Prepare(t.Context(), plugins.CallRequest{Installation: installation.ID(), Digest: release.Digest(), Contribution: "memory-provider", Contract: contracts.Capability, Method: rpc.Add, Input: addition, Target: capability.Target{Scope: "installation", Reference: installation.ID().String()}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publicAdd.Invoke(addContext); err != nil {
		t.Fatal(err)
	}
	stored, found, err := host.PluginCommands.Get(t.Context(), operation.Identity(addContext))
	if err != nil || !found {
		t.Fatalf("original add route: found=%v, %v", found, err)
	}
	request := plugins.CallRequest{Installation: installation.ID(), Digest: release.Digest(), Contribution: "memory-provider", Contract: contracts.Capability, Method: rpc.Add, Input: stored.Snapshot().Input, Target: stored.Snapshot().Invocation.Target}
	installed, err := calls.Prepare(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	beforeRecovery := len(notices)
	if _, err := installed.Invoke(addContext); err != nil {
		t.Fatalf("installed entrance did not recover the feature owner's receipt: %v", err)
	}
	if len(notices) != beforeRecovery+1 || notices[len(notices)-1].Resource != invalidation.PluginData {
		t.Fatalf("public command recovery did not invalidate its consumer reads: %+v", notices)
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
	if len(notices) != beforeRecovery+1 {
		t.Fatal("rejected original-binding conflicts published change notices")
	}
	unconfirmedContext := operation.Bind(t.Context(), "memory.add", "unconfirmed")
	unconfirmedRecord := stored.Snapshot()
	unconfirmedRecord.ID = operation.Identity(unconfirmedContext)
	unconfirmedRecord.Invocation, err = operation.Capture(unconfirmedContext, unconfirmedRecord.Invocation.Target)
	if err != nil {
		t.Fatal(err)
	}
	unconfirmed, err := plugin.NewCommand(unconfirmedRecord)
	if err != nil {
		t.Fatal(err)
	}
	if err := host.PluginCommands.Save(t.Context(), unconfirmed); err != nil {
		t.Fatal(err)
	}
	if err := routing.Dispatch(t.Context(), unconfirmedRecord.ID, func(context.Context, plugin.CommandBinding) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := installed.Invoke(unconfirmedContext); !errors.Is(err, plugin.ErrCommandUnconfirmed) {
		t.Fatalf("missing original receipt was not retained as uncertainty: %v", err)
	}
	if len(notices) != beforeRecovery+2 || notices[len(notices)-1].Resource != invalidation.PluginData {
		t.Fatal("unconfirmed original command did not invalidate consumer reads")
	}
	items, err := owner.Search(t.Context(), project, "keyword", 5)
	if err != nil || len(items) != 1 || items[0].Content != "keyword fact" {
		t.Fatalf("absent role lost keyword search: %v, %v", items, err)
	}
	current, err := owner.Items(t.Context(), "project", project)
	if err != nil || len(current) != 1 || current[0].Content != "keyword fact" || current[0].ID != items[0].ID || current[0].UpdatedAt.IsZero() {
		t.Fatalf("published active read changed prompt values: %+v, %v", current, err)
	}
	embedder := &ownedMemoryEmbedder{t: t, parent: "original-memory-query"}
	owner.embedder = func(context.Context) (models.TextEmbedder, error) { return embedder, nil }
	items, err = owner.Search(operation.Adopt(t.Context(), "original-memory-query", capability.Model, "executing-owner"), project, "keyword", 5)
	if err != nil || len(items) != 1 || embedder.calls.Load() != 2 {
		t.Fatalf("human search callback metadata: items=%v, embeddings=%d, %v", items, embedder.calls.Load(), err)
	}
	failure := errors.New("installed memory read failed")
	store.failure = failure
	if items, err := owner.Items(t.Context(), "project", project); !errors.Is(err, failure) || len(items) != 0 {
		t.Fatalf("operational failure became absent memory: %+v, %v", items, err)
	}
	store.failure = nil
	store.snapshot.Installation.Disable()
	if items, err := owner.Items(t.Context(), "project", project); err != nil || len(items) != 0 {
		t.Fatalf("optional absence failed: %+v, %v", items, err)
	}
}
