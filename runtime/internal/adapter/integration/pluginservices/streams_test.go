package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type streamFixture struct {
	calls       *CommandCalls
	request     plugins.CallRequest
	store       *toolInstallationStore
	host        *persistencefixture.Stores
	managed     *managedFixture
	invocations atomic.Int32
	mu          sync.Mutex
	receipt     qimumcp.Receipt
	digest      string
}

type streamInput struct {
	Revision int64 `json:"revision,omitempty"`
}

func TestInstalledCommandBindingPersistenceFailurePreventsDispatch(t *testing.T) {
	f := newStreamFixture(t, "complete")
	if _, err := f.host.ExecContext(t.Context(), `CREATE TRIGGER refuse_command BEFORE INSERT ON plugin_commands BEGIN SELECT RAISE(ABORT,'binding storage refused'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.run(t, operation.Bind(t.Context(), "records.stream", "storage-failure"))
	if err == nil || !strings.Contains(err.Error(), "binding storage refused") || f.invocations.Load() != 0 {
		t.Fatalf("binding failure escaped admission: calls=%d, %v", f.invocations.Load(), err)
	}
	var count int
	if err := f.host.QueryRowContext(t.Context(), `SELECT count(*) FROM plugin_commands`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed binding left a command: %d, %v", count, err)
	}
}

func TestMalformedStreamInputIsRefusedBeforeBindingAndLeavesIdentityAvailable(t *testing.T) {
	f := newStreamFixture(t, "complete")
	ctx := operation.Bind(t.Context(), "records.stream", "malformed-input")
	original := f.request.Input
	malformed, err := jsonvalue.Parse([]byte(`{"unexpected":true}`))
	if err != nil {
		t.Fatal(err)
	}
	f.request.Input = malformed
	events, err := f.run(t, ctx)
	if !errors.Is(err, plugin.ErrInvalid) || errors.Is(err, plugin.ErrCommandUnconfirmed) || len(events) != 0 || f.invocations.Load() != 0 {
		t.Fatalf("malformed stream entered execution or uncertainty: events=%+v calls=%d, %v", events, f.invocations.Load(), err)
	}
	var bindings int
	if err := f.host.QueryRowContext(t.Context(), `SELECT count(*) FROM plugin_commands`).Scan(&bindings); err != nil || bindings != 0 {
		t.Fatalf("malformed stream retained a command binding: %d, %v", bindings, err)
	}
	f.request.Input = original
	events, err = f.run(t, ctx)
	if err != nil || len(events) != 2 || events[0].Kind != plugins.StreamChunk || events[1].Kind != plugins.StreamResult || f.invocations.Load() != 1 {
		t.Fatalf("malformed stream consumed the original identity: events=%+v calls=%d, %v", events, f.invocations.Load(), err)
	}
}

func TestTypedStreamInputIsRefusedBeforeBindingAndLeavesIdentityAvailable(t *testing.T) {
	for _, mode := range []string{"complete", "query"} {
		t.Run(mode, func(t *testing.T) {
			f := newStreamFixture(t, mode)
			ctx := operation.Bind(t.Context(), "records.stream", "typed-input")
			original := f.request.Input
			for _, number := range []string{"0.0", "1e0"} {
				input, err := jsonvalue.Parse([]byte(`{"revision":` + number + `}`))
				if err != nil {
					t.Fatal(err)
				}
				method, _ := f.request.Contract.Method(f.request.Method)
				if err := method.ValidateInput(input); err != nil {
					t.Fatalf("probe must reach the complete typed admission owner: %v", err)
				}
				f.request.Input = input
				events, err := f.run(t, ctx)
				if !errors.Is(err, plugin.ErrInvalid) || errors.Is(err, plugin.ErrCommandUnconfirmed) || len(events) != 0 || f.invocations.Load() != 0 {
					t.Fatalf("typed-invalid stream entered execution or uncertainty: events=%+v calls=%d, %v", events, f.invocations.Load(), err)
				}
				var bindings int
				if err := f.host.QueryRowContext(t.Context(), `SELECT count(*) FROM plugin_commands`).Scan(&bindings); err != nil || bindings != 0 {
					t.Fatalf("typed-invalid stream retained a command binding: %d, %v", bindings, err)
				}
			}
			f.request.Input = original
			events, err := f.run(t, ctx)
			if err != nil || len(events) != 2 || events[0].Kind != plugins.StreamChunk || events[1].Kind != plugins.StreamResult || f.invocations.Load() != 1 {
				t.Fatalf("typed-invalid stream consumed the original identity: events=%+v calls=%d, %v", events, f.invocations.Load(), err)
			}
		})
	}
}

func newStreamFixture(t *testing.T, mode string) *streamFixture {
	t.Helper()
	f := &streamFixture{}
	kind := portable.Mutation
	if mode == "query" {
		kind = portable.Query
	}
	method, err := qimumcp.NewStreamingMethod[streamInput, string, bool](qimumcp.MethodConfig{Name: "records.stream", Kind: kind, Target: extension.ScopeInstallation, Description: "Stream owned records."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "records", Composition: portable.Selection, Scope: extension.ScopeInstallation}, method.Descriptor())
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
	backend, err := qimumcp.NewBackend(remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := method.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[streamInput]) iter.Seq2[qimumcp.StreamValue[string, bool], error] {
		return func(yield func(qimumcp.StreamValue[string, bool], error) bool) {
			f.invocations.Add(1)
			if mode == "query" && command.ID != "" {
				yield(qimumcp.StreamValue[string, bool]{}, errors.New("query carried a command identity"))
				return
			}
			if !yield(qimumcp.StreamValue[string, bool]{Chunk: new("known partial")}, nil) {
				return
			}
			if mode == "query" {
				yield(qimumcp.StreamValue[string, bool]{Result: new(true)}, nil)
				return
			}
			if mode == "unknown" {
				yield(qimumcp.StreamValue[string, bool]{}, errors.New("lost owner outcome"))
				return
			}
			if mode == "refusal" {
				input, err := command.RequestInput()
				if err != nil {
					yield(qimumcp.StreamValue[string, bool]{}, err)
					return
				}
				f.mu.Lock()
				f.digest = input.Digest()
				f.receipt = qimumcp.Receipt{Found: true, Reply: []byte(`{"failure":{"code":"refused","message":"owner refused"}}`)}
				f.mu.Unlock()
				yield(qimumcp.StreamValue[string, bool]{}, &portable.Refusal{Code: "refused", Message: "owner refused"})
				return
			}
			input, err := command.RequestInput()
			if err != nil {
				yield(qimumcp.StreamValue[string, bool]{}, err)
				return
			}
			f.mu.Lock()
			f.digest = input.Digest()
			f.receipt = qimumcp.Receipt{Found: true, Reply: []byte(`{"value":true}`)}
			f.mu.Unlock()
			yield(qimumcp.StreamValue[string, bool]{Result: new(true)}, nil)
		}
	}); err != nil {
		t.Fatal(err)
	}
	lookup, err := qimumcp.ReceiptQuery()
	if err != nil {
		t.Fatal(err)
	}
	if err := lookup.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (qimumcp.Receipt, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.receipt.Found && command.Input.InputDigest != f.digest {
			return qimumcp.Receipt{}, errors.New("original input changed")
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
	callbacks := qimumcp.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	if err := callbacks.PublishRPC(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected host call", http.StatusBadRequest)
	})); err != nil {
		t.Fatal(err)
	}
	f.managed = newManagedFixture(t, f.store, server, callbacks, nil)
	f.host = f.managed.Stores
	f.calls, err = NewCommandCalls(f.managed.Host, f.store, f.host.ProviderSelections, f.host.PluginCompositions, f.managed.Pool, f.managed.Routing, nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Encode(struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	f.request = plugins.CallRequest{Installation: installation.ID(), Digest: release.Digest(), Contribution: "records", Contract: contract, Method: "records.stream", Input: input, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installation.ID().String()}}
	return f
}

func (f *streamFixture) reopen(t *testing.T) {
	t.Helper()
	f.managed.reopen(t)
	f.host = f.managed.Stores
	var err error
	f.calls, err = NewCommandCalls(f.managed.Host, f.store, f.host.ProviderSelections, f.host.PluginCompositions, f.managed.Pool, f.managed.Routing, nil)
	if err != nil {
		t.Fatal(err)
	}
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
	row, found, err := f.host.PluginCommands.Get(t.Context(), operation.Identity(ctx))
	if err != nil || !found || row.Snapshot().Phase != plugin.CommandObserved {
		t.Fatalf("original route=%+v %v", row, err)
	}
	f.reopen(t)
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
	row, _, err := f.host.PluginCommands.Get(t.Context(), operation.Identity(ctx))
	if err != nil || row.Snapshot().Phase != plugin.CommandDispatched {
		t.Fatalf("uncertainty retired: %+v %v", row, err)
	}
}

func TestInstalledStreamKnownRefusalObservesAndRestoresOriginalReceipt(t *testing.T) {
	f := newStreamFixture(t, "refusal")
	ctx := operation.Bind(t.Context(), "records.stream", "refused")
	for range 2 {
		_, err := f.run(t, ctx)
		if refusal, known := errors.AsType[*portable.Refusal](err); !known || refusal.Code != "refused" || errors.Is(err, plugin.ErrCommandUnconfirmed) || errors.Is(err, qimumcp.ErrStreamUnconfirmed) {
			t.Fatalf("known refusal became uncertain: %v", err)
		}
	}
	row, _, err := f.host.PluginCommands.Get(t.Context(), operation.Identity(ctx))
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
	changed := operation.Adopt(t.Context(), operation.Identity(ctx), extension.Automatic, "")
	if _, err := f.run(t, changed); !errors.Is(err, portable.ErrInvocationConflict) {
		t.Fatalf("changed origin admitted: %v", err)
	}
	input, err := jsonvalue.Parse([]byte(`{"changed":true}`))
	if err != nil {
		t.Fatal(err)
	}
	f.request.Input = input
	if _, err := f.run(t, ctx); !errors.Is(err, portable.ErrInvocationConflict) || f.invocations.Load() != 1 {
		t.Fatalf("changed input admitted: %v calls=%d", err, f.invocations.Load())
	}
}

func TestInstalledStreamWithdrawalCannotReplayAtAnotherOwner(t *testing.T) {
	f := newStreamFixture(t, "unknown")
	ctx := operation.Bind(t.Context(), "records.stream", "withdrawn")
	if _, err := f.run(t, ctx); !errors.Is(err, plugin.ErrCommandUnconfirmed) {
		t.Fatal(err)
	}
	f.store.disable(t)
	if _, err := f.run(t, ctx); !errors.Is(err, plugin.ErrCommandUnconfirmed) || !errors.Is(err, plugin.ErrUnavailable) || f.invocations.Load() != 1 {
		t.Fatalf("withdrawn original retargeted: %v calls=%d", err, f.invocations.Load())
	}
}

func TestInstalledQueryStreamHasNoCommandRouteOrReceipt(t *testing.T) {
	f := newStreamFixture(t, "query")
	call, err := f.calls.PrepareStream(t.Context(), f.request)
	if err != nil {
		t.Fatal(err)
	}
	if call.Kind() != portable.Query {
		t.Fatal("query stream became a command")
	}
	for range 2 {
		ctx := operation.Start(t.Context(), extension.Human, "query-entrance", "")
		var events []plugins.StreamEvent
		for event, err := range call.Stream(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			events = append(events, event)
		}
		if len(events) != 2 || events[0].Kind != plugins.StreamChunk || events[1].Kind != plugins.StreamResult || string(events[1].Value.JSON()) != "true" {
			t.Fatalf("query stream = %+v", events)
		}
	}
	var commands int
	if err := f.host.QueryRowContext(t.Context(), "SELECT count(*) FROM plugin_commands").Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if commands != 0 {
		t.Fatalf("query created %d durable command routes", commands)
	}
	dependencies, err := f.host.PluginCommands.PendingCommandDependencies(t.Context())
	if err != nil || len(dependencies) != 0 || f.receipt.Found || f.invocations.Load() != 2 {
		t.Fatalf("query acquired command recovery evidence: dependencies=%v receipt=%+v invocations=%d error=%v", dependencies, f.receipt, f.invocations.Load(), err)
	}
}

func TestPreparedCommandRefusesChangedSetupWithTheSameReleases(t *testing.T) {
	f := newStreamFixture(t, "complete")
	ctx := operation.Bind(t.Context(), "records.stream", "original-graph")
	call, err := f.calls.PrepareStream(ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	prepared := call.(*installedStream)
	record, err := f.calls.admitCommandRoute(ctx, f.request.Method, f.request.Input, prepared.accepts, prepared.resolve, func(plugins.ContributionBinding) portable.Target { return f.request.Target }, true)
	if err != nil {
		t.Fatal(err)
	}
	changed := record
	binding, err := plugins.DecodeActivation(record.Activation)
	if err != nil {
		t.Fatal(err)
	}
	binding.Units[0].Configuration = qimu.Hash([]byte("another setup in the same release"))
	changed.Activation, err = plugins.ProjectActivation(binding)
	if err != nil {
		t.Fatal(err)
	}
	admit := func(record plugin.CommandBinding) error {
		return f.host.Admission.Admit(ctx, func(ctx context.Context) error { return f.calls.admitDispatch(ctx, record) })
	}
	if err := admit(changed); !errors.Is(err, qimu.ErrStaleBinding) {
		t.Fatalf("same-release graph change admitted: %v", err)
	}
	if err := admit(record); err != nil {
		t.Fatalf("original graph refused: %v", err)
	}
	if f.invocations.Load() != 0 {
		t.Fatal("graph validation dispatched an operation")
	}
}
