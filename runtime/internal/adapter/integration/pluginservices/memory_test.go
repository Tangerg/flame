package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
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
	invocation, err := qimumcp.InvocationFromContext(ctx)
	if err != nil || invocation.Origin != extension.Model || invocation.Parent != e.parent || invocation.ID == e.parent {
		e.t.Errorf("Memory embedding detached from its original query: %+v, %v", invocation, err)
	}
	return e.preparedEmbedder.Embed(ctx, texts)
}

func TestMemoryMaintenanceCapturesBusinessInputAndRecoversBackendReceipts(t *testing.T) {
	name := testsupport.ServerName("memory")
	// The Host exercises the published stdio boundary. Private Memory storage
	// and publication invariants are verified by the backend's own tests.
	executable := filepath.Join(t.TempDir(), "flame-memory")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", executable, "./backend")
	build.Dir = filepath.Join("..", "..", "..", "..", "..", "plugins", "memory")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Memory backend: %v\n%s", err, output)
	}
	environment := map[string]string{"PLUGIN_DATA": t.TempDir()}
	contract, err := portable.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.memory", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "./flame-memory"}}, Contributions: []plugin.Contribution{{ID: "memory-provider", Server: name, Export: "memory", Contract: contract.Identity()}}})
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
	server := mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStdio, Command: executable, Env: environment}
	callbacks := qimumcp.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	managed := newManagedFixture(t, store, server, callbacks, nil)
	pool := managed.Pool
	host := managed.Stores
	selections := host.ProviderSelections
	choice, err := plugin.NewProviderSelection(plugin.SelectionRecord{Role: contract.Descriptor().Reference, Installation: installation.ID(), Contribution: "memory-provider"})
	if err != nil {
		t.Fatal(err)
	}
	if err := selections.Save(t.Context(), choice); err != nil {
		t.Fatal(err)
	}
	admission := host.Admission
	routing, err := plugins.NewCommands(host.PluginCommands, admission)
	if err != nil {
		t.Fatal(err)
	}
	var notices []invalidation.Notice
	publication := invalidation.Publish(func(notice invalidation.Notice) { notices = append(notices, notice) })
	calls, err := NewCommandCalls(managed.Host, store, selections, host.PluginCompositions, pool, routing, publication)
	if err != nil {
		t.Fatal(err)
	}
	observers, err := NewCallComposition(calls, callbacks, admission)
	if err != nil {
		t.Fatal(err)
	}
	var modelCalls, releases atomic.Int32
	modelInvocations := make(chan portable.Invocation, 2)
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		invocation, err := qimumcp.InvocationFromContext(ctx)
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
	project, err := (filesystem.DirectoryResolver{}).ResolveExistingDir(t.TempDir())
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
	var id, hostInput, phase string
	if err := host.QueryRowContext(t.Context(), `SELECT id,business_input_json,phase FROM plugin_commands WHERE method=?`, memorycontract.MaintainV1Method).Scan(&id, &hostInput, &phase); err != nil {
		t.Fatal(err)
	}
	first, second := <-modelInvocations, <-modelInvocations
	if first.ID == second.ID || first.Parent != id || second.Parent != id || first.Origin != extension.Automatic || second.Origin != extension.Automatic || first.Target.Scope != extension.ScopeCall || second.Target.Scope != extension.ScopeCall {
		t.Fatalf("model calls lost distinct original ancestry: %+v, %+v", first, second)
	}
	var input memorycontract.MaintainV1Input
	if err := json.Unmarshal([]byte(hostInput), &input, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if input.SessionID != "ses_metadata" || input.Project != project || !input.Compacted || input.Conversation.MessageCount != int64(len(history)) || input.Conversation.Transcript == "" || phase != string(plugin.CommandObserved) {
		t.Fatalf("original business-only routing: %+v, phase=%s", input, phase)
	}
	borrowed, err := pool.BorrowSession(t.Context(), server.ID())
	if err != nil {
		t.Fatal(err)
	}
	recorded, found, err := host.PluginCommands.Get(t.Context(), id)
	if err != nil || !found {
		t.Fatalf("original maintenance command: %v, %v", found, err)
	}
	defer borrowed.Close()
	bound, err := qimumcp.NewCapabilityBinding(borrowed.Session(), "memory", contract)
	if err != nil {
		t.Fatal(err)
	}
	maintain, err := bound.Method(memorycontract.MaintainV1Method)
	if err != nil {
		t.Fatal(err)
	}
	receiptContext, err := qimumcp.WithInvocation(borrowed.Context(), recorded.Snapshot().Invocation)
	if err != nil {
		t.Fatal(err)
	}
	receipt, found, err := maintain.ReadReceipt(receiptContext, id, recorded.Snapshot().Input)
	if err != nil || !found {
		t.Fatalf("original backend receipt: %v, %v", found, err)
	}
	if err := borrowed.Close(); err != nil {
		t.Fatal(err)
	}
	var outcome memorycontract.MaintainV1Output
	if err := json.Unmarshal(receipt.JSON(), &outcome, json.RejectUnknownMembers(true)); err != nil || !outcome.Published {
		t.Fatalf("maintenance receipt lost its published outcome: %+v, %v", outcome, err)
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
	addition, err := jsonvalue.Encode(memorycontract.AddV2Input{Scope: memorycontract.AddV2InputScopeProject, CWD: project, Content: "keyword fact"})
	if err != nil {
		t.Fatal(err)
	}
	publicAdd, err := calls.Prepare(t.Context(), plugins.CallRequest{Installation: installation.ID(), Digest: release.Digest(), Contribution: "memory-provider", Contract: contract, Method: memorycontract.AddV2Method, Input: addition, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installation.ID().String()}})
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
	request := plugins.CallRequest{Installation: installation.ID(), Digest: release.Digest(), Contribution: "memory-provider", Contract: contract, Method: memorycontract.AddV2Method, Input: stored.Snapshot().Input, Target: stored.Snapshot().Invocation.Target}
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
	if _, err := installed.Invoke(operation.Adopt(t.Context(), operation.Identity(addContext), extension.Model, "parent-tool")); !errors.Is(err, portable.ErrInvocationConflict) {
		t.Fatalf("changed calling origin reused an original identity: %v", err)
	}
	changed := request
	changed.Contribution = "another-provider"
	other, err := calls.Prepare(t.Context(), changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Invoke(addContext); !errors.Is(err, portable.ErrInvocationConflict) {
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
	if err := admission.Admit(t.Context(), func(ctx context.Context) error {
		binding, err := plugins.DecodeActivation(unconfirmedRecord.Activation)
		if err != nil {
			return err
		}
		if err := managed.Host.ValidateBindingUnder(ctx, binding); err != nil {
			return err
		}
		return host.PluginCommands.Save(ctx, unconfirmed)
	}); err != nil {
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
	current, err := owner.Items(t.Context(), memorycontract.ItemsV2InputScopeProject, project)
	if err != nil || len(current) != 1 || current[0].Content != "keyword fact" || current[0].ID != items[0].ID || current[0].UpdatedAt.IsZero() {
		t.Fatalf("published active read changed prompt values: %+v, %v", current, err)
	}
	embedder := &ownedMemoryEmbedder{t: t, parent: "original-memory-query"}
	owner.embedder = func(context.Context) (models.TextEmbedder, error) { return embedder, nil }
	items, err = owner.Search(operation.Adopt(t.Context(), "original-memory-query", extension.Model, "executing-owner"), project, "keyword", 5)
	if err != nil || len(items) != 1 || embedder.calls.Load() != 2 {
		t.Fatalf("human search callback metadata: items=%v, embeddings=%d, %v", items, embedder.calls.Load(), err)
	}
	failure := errors.New("installed memory read failed")
	store.failure = failure
	if items, err := owner.Items(t.Context(), memorycontract.ItemsV2InputScopeProject, project); !errors.Is(err, failure) || len(items) != 0 {
		t.Fatalf("operational failure became absent memory: %+v, %v", items, err)
	}
	store.failure = nil
	store.disable(t)
	if items, err := owner.Items(t.Context(), memorycontract.ItemsV2InputScopeProject, project); err != nil || len(items) != 0 {
		t.Fatalf("optional absence failed: %+v, %v", items, err)
	}
}
