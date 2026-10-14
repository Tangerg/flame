package pluginservices

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	hostsqlite "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type streamFixture struct {
	calls       *CommandCalls
	request     plugins.CallRequest
	store       *toolInstallationStore
	host        *sql.DB
	hostPath    string
	invocations atomic.Int32
	mu          sync.Mutex
	receipt     pluginsdk.Receipt
	digest      string
}

func newStreamFixture(t *testing.T, mode string) *streamFixture {
	t.Helper()
	f := &streamFixture{}
	method, err := pluginsdk.NewStreamingMethod[struct{}, string, bool](pluginsdk.MethodConfig{Name: "records.stream", Kind: capability.Mutation, Target: "installation", Description: "Stream owned records."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := capability.New(capability.Config{Reference: "records", Composition: capability.Selection, Scope: "installation"}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	name := testsupport.ServerName("records")
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.records", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "backend"}}, Contributions: []plugin.Contribution{{ID: "records", Server: name, Export: "records", Contract: contract.Identity()}}})
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
	f.store = &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	remote := sdk.NewServer(&sdk.Implementation{Name: "streaming-records"}, nil)
	backend, err := pluginsdk.NewBackend(remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[struct{}]) iter.Seq2[pluginsdk.StreamValue[string, bool], error] {
		return func(yield func(pluginsdk.StreamValue[string, bool], error) bool) {
			f.invocations.Add(1)
			if !yield(pluginsdk.StreamValue[string, bool]{Chunk: new("known partial")}, nil) {
				return
			}
			if mode == "unknown" {
				yield(pluginsdk.StreamValue[string, bool]{}, errors.New("lost owner outcome"))
				return
			}
			if mode == "refusal" {
				input, err := command.RequestInput()
				if err != nil {
					yield(pluginsdk.StreamValue[string, bool]{}, err)
					return
				}
				f.mu.Lock()
				f.digest = input.Digest()
				f.receipt = pluginsdk.Receipt{Found: true, Reply: []byte(`{"failure":{"code":"refused","message":"owner refused"}}`)}
				f.mu.Unlock()
				yield(pluginsdk.StreamValue[string, bool]{}, &capability.Refusal{Code: "refused", Message: "owner refused"})
				return
			}
			input, err := command.RequestInput()
			if err != nil {
				yield(pluginsdk.StreamValue[string, bool]{}, err)
				return
			}
			f.mu.Lock()
			f.digest = input.Digest()
			f.receipt = pluginsdk.Receipt{Found: true, Reply: []byte(`{"value":true}`)}
			f.mu.Unlock()
			yield(pluginsdk.StreamValue[string, bool]{Result: new(true)}, nil)
		}
	}); err != nil {
		t.Fatal(err)
	}
	lookup, err := pluginsdk.NewMethod[pluginsdk.ReceiptRequest, pluginsdk.Receipt](pluginsdk.MethodConfig{Name: pluginsdk.ReceiptMethod, Kind: capability.Query, Target: "installation", Description: "Read the original private receipt."})
	if err != nil {
		t.Fatal(err)
	}
	if err := lookup.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ReceiptRequest]) (pluginsdk.Receipt, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.receipt.Found && command.Input.InputDigest != f.digest {
			return pluginsdk.Receipt{}, errors.New("original input changed")
		}
		return f.receipt, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("records", contract); err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(endpoint.Close)
	server := mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: endpoint.URL}
	callbacks := pluginsdk.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	if err := callbacks.PublishRPC(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected host call", http.StatusBadRequest)
	})); err != nil {
		t.Fatal(err)
	}
	pool, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, toolSourceRegistry{server}, callbacks, nil, nil, nil)
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
	f.hostPath = filepath.Join(t.TempDir(), "host.db")
	f.host, err = hostsqlite.Open(t.Context(), f.hostPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.host.Close() })
	routing, err := plugins.NewCommands(hostsqlite.NewPluginCommandStore(f.host), plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	f.calls, err = NewCommandCalls(f.store, hostsqlite.NewProviderSelectionStore(f.host), hostsqlite.NewCompositionStore(f.host), pool, routing, nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsoninput.Encode(struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	f.request = plugins.CallRequest{Installation: installation.ID(), Digest: release.Digest(), Contribution: "records", Contract: contract, Method: "records.stream", Input: input, Target: capability.Target{Scope: "installation", Reference: installation.ID().String()}}
	return f
}

func (f *streamFixture) run(t *testing.T, ctx context.Context) ([]plugins.StreamEvent, error) {
	t.Helper()
	call, err := f.calls.PrepareStream(ctx, f.request)
	if err != nil {
		return nil, err
	}
	var events []plugins.StreamEvent
	for value, err := range call.Stream(ctx) {
		if err != nil {
			return events, err
		}
		events = append(events, value)
	}
	return events, nil
}

func TestInstalledStreamObservesFinalAndRestoresOnlyOriginalReceipt(t *testing.T) {
	f := newStreamFixture(t, "complete")
	ctx := operation.Bind(t.Context(), "records.stream", "original")
	events, err := f.run(t, ctx)
	if err != nil || len(events) != 2 || events[0].Kind != plugins.StreamChunk || events[1].Kind != plugins.StreamResult {
		t.Fatalf("installed stream=%+v %v", events, err)
	}
	row, found, err := hostsqlite.NewPluginCommandStore(f.host).Get(t.Context(), operation.Identity(ctx))
	if err != nil || !found || row.Snapshot().Phase != plugin.CommandObserved {
		t.Fatalf("original route=%+v %v", row, err)
	}
	if err := f.host.Close(); err != nil {
		t.Fatal(err)
	}
	f.host, err = hostsqlite.Open(t.Context(), f.hostPath)
	if err != nil {
		t.Fatal(err)
	}
	routing, err := plugins.NewCommands(hostsqlite.NewPluginCommandStore(f.host), plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	f.calls, err = NewCommandCalls(f.store, hostsqlite.NewProviderSelectionStore(f.host), hostsqlite.NewCompositionStore(f.host), f.calls.pool, routing, nil)
	if err != nil {
		t.Fatal(err)
	}
	events, err = f.run(t, ctx)
	if err != nil || len(events) != 1 || events[0].Kind != plugins.StreamResult || f.invocations.Load() != 1 {
		t.Fatalf("receipt restore repeated stream: %+v %v calls=%d", events, err, f.invocations.Load())
	}
	var result bool
	if err := json.Unmarshal(events[0].Value.JSON(), &result); err != nil || !result {
		t.Fatalf("original result lost: %v %v", result, err)
	}
}

func TestInstalledStreamMissingReceiptNeverResubmits(t *testing.T) {
	f := newStreamFixture(t, "unknown")
	ctx := operation.Bind(t.Context(), "records.stream", "unconfirmed")
	events, err := f.run(t, ctx)
	if len(events) != 1 || !errors.Is(err, plugin.ErrCommandUnconfirmed) {
		t.Fatalf("partial outcome=%+v %v", events, err)
	}
	for range 2 {
		if _, err := f.run(t, ctx); !errors.Is(err, plugin.ErrCommandUnconfirmed) {
			t.Fatalf("missing receipt became success: %v", err)
		}
	}
	if f.invocations.Load() != 1 {
		t.Fatalf("original stream resubmitted %d times", f.invocations.Load())
	}
	row, _, err := hostsqlite.NewPluginCommandStore(f.host).Get(t.Context(), operation.Identity(ctx))
	if err != nil || row.Snapshot().Phase != plugin.CommandDispatched {
		t.Fatalf("uncertainty retired: %+v %v", row, err)
	}
}

func TestInstalledStreamKnownRefusalObservesAndRestoresOriginalReceipt(t *testing.T) {
	f := newStreamFixture(t, "refusal")
	ctx := operation.Bind(t.Context(), "records.stream", "refused")
	for range 2 {
		_, err := f.run(t, ctx)
		if refusal, known := errors.AsType[*capability.Refusal](err); !known || refusal.Code != "refused" || errors.Is(err, plugin.ErrCommandUnconfirmed) || errors.Is(err, pluginsdk.ErrStreamUnconfirmed) {
			t.Fatalf("known refusal became uncertain: %v", err)
		}
	}
	row, _, err := hostsqlite.NewPluginCommandStore(f.host).Get(t.Context(), operation.Identity(ctx))
	if err != nil || row.Snapshot().Phase != plugin.CommandObserved || f.invocations.Load() != 1 {
		t.Fatalf("known refusal route=%+v %v calls=%d", row, err, f.invocations.Load())
	}
}

func TestInstalledStreamConsumerStopRetainsUnconfirmedOriginal(t *testing.T) {
	f := newStreamFixture(t, "complete")
	ctx := operation.Bind(t.Context(), "records.stream", "stop")
	call, err := f.calls.PrepareStream(ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	for event, err := range call.Stream(ctx) {
		if err != nil || event.Kind != plugins.StreamChunk {
			t.Fatalf("first chunk=%+v %v", event, err)
		}
		break
	}
	if _, err := f.run(t, ctx); !errors.Is(err, plugin.ErrCommandUnconfirmed) || f.invocations.Load() != 1 {
		t.Fatalf("stopped original was replayed: %v calls=%d", err, f.invocations.Load())
	}
}

func TestInstalledStreamOriginalOriginAndInputCannotChange(t *testing.T) {
	f := newStreamFixture(t, "unknown")
	ctx := operation.Bind(t.Context(), "records.stream", "captured")
	if _, err := f.run(t, ctx); !errors.Is(err, plugin.ErrCommandUnconfirmed) {
		t.Fatal(err)
	}
	changed := operation.Adopt(t.Context(), operation.Identity(ctx), capability.Automatic, "")
	if _, err := f.run(t, changed); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("changed origin admitted: %v", err)
	}
	input, err := jsoninput.Parse([]byte(`{"changed":true}`))
	if err != nil {
		t.Fatal(err)
	}
	f.request.Input = input
	if _, err := f.run(t, ctx); !errors.Is(err, idempotency.ErrKeyConflict) || f.invocations.Load() != 1 {
		t.Fatalf("changed input admitted: %v calls=%d", err, f.invocations.Load())
	}
}

func TestInstalledStreamWithdrawalCannotReplayAtAnotherOwner(t *testing.T) {
	f := newStreamFixture(t, "unknown")
	ctx := operation.Bind(t.Context(), "records.stream", "withdrawn")
	if _, err := f.run(t, ctx); !errors.Is(err, plugin.ErrCommandUnconfirmed) {
		t.Fatal(err)
	}
	f.store.snapshot.Installation.Disable()
	if _, err := f.run(t, ctx); !errors.Is(err, plugin.ErrCommandUnconfirmed) || !errors.Is(err, plugin.ErrUnavailable) || f.invocations.Load() != 1 {
		t.Fatalf("withdrawn original retargeted: %v calls=%d", err, f.invocations.Load())
	}
}
