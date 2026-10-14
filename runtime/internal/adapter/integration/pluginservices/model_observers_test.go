package pluginservices

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	hostsqlite "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestInstalledModelObserverKeepsKnownOutcomeAndSingleOriginalContinuation(t *testing.T) {
	for _, mode := range []string{"transparent", "outer-failure", "repeat", "transform"} {
		t.Run(mode, func(t *testing.T) {
			observer, bindings, store := installedObserverFixture(t, mode)
			var calls atomic.Int32
			message := chat.NewAssistantMessage(chat.NewTextPart("known response"), chat.NewReasoningPart("basis", nil))
			response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "actual-endpoint", Usage: &chat.Usage{InputTokens: 11, OutputTokens: 3}}}
			model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) { calls.Add(1); return response, nil })
			bound, streamer := observer.Bind(model, nil, bindings)
			if streamer != nil {
				t.Fatal("observer fabricated streaming capability")
			}
			if _, advertised := bound.(chat.Streamer); advertised {
				t.Fatal("unary provider advertised a fabricated stream capability")
			}
			ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-model", Origin: capability.Automatic, Parent: "original-effect", Target: capability.Target{Scope: "call", Reference: "original-model"}})
			if err != nil {
				t.Fatal(err)
			}
			request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := bound.Call(ctx, request)
			if mode == "transform" {
				if err == nil || actual != nil || calls.Load() != 0 {
					t.Fatalf("transformed input reached provider: %+v, %d, %v", actual, calls.Load(), err)
				}
			} else {
				if !reflect.DeepEqual(actual, response) || calls.Load() != 1 || (err == nil) != (mode == "transparent") {
					t.Fatalf("inner outcome changed: %+v, %d, %v", actual, calls.Load(), err)
				}
			}
			store.snapshot.Installation.Disable()
			if err := observer.Validate(ctx, bindings); err == nil {
				t.Fatal("withdrawn observer was admitted for restoration")
			}
			before := calls.Load()
			if _, err := bound.Call(ctx, request); err == nil || calls.Load() != before {
				t.Fatalf("frozen observer survived withdrawal: %d, %v", calls.Load(), err)
			}
		})
	}
}

func TestInstalledModelObserverStreamsCanonicalChunksThroughCapturedNext(t *testing.T) {
	for _, mode := range []string{"transparent", "outer-failure"} {
		t.Run(mode, func(t *testing.T) {
			observer, bindings, _ := installedObserverFixture(t, mode)
			model := &observerStreamModel{done: make(chan struct{})}
			_, streamer := observer.Bind(model, model, bindings)
			ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-stream", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "original-stream"}})
			if err != nil {
				t.Fatal(err)
			}
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
			sequence := streamer.Stream(ctx, request)
			if model.calls.Load() != 0 {
				t.Fatal("unconsumed stream invoked provider")
			}
			var accumulated chat.ResponseAccumulator
			var failure error
			for delta, err := range sequence {
				if err != nil {
					failure = err
					break
				}
				if err := accumulated.Add(delta); err != nil {
					t.Fatal(err)
				}
				if accumulatedText, _ := accumulated.Response(); accumulatedText == nil && model.produced.Load() != 1 {
					t.Fatal("provider advanced before first chunk consumption")
				}
			}
			response, err := accumulated.Response()
			if err != nil || response.Text() != "canonical stream" || response.Metadata.Usage.InputTokens != 7 || model.calls.Load() != 1 || (failure == nil) != (mode == "transparent") {
				t.Fatalf("stream evidence changed: %+v, %d, %v, %v", response, model.calls.Load(), err, failure)
			}
		})
	}
}

type observerStreamModel struct {
	calls    atomic.Int32
	produced atomic.Int32
	done     chan struct{}
}

func (*observerStreamModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, errors.New("unary is not used")
}

func (m *observerStreamModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		defer close(m.done)
		m.calls.Add(1)
		m.produced.Add(1)
		if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("canonical ")}}, nil) {
			return
		}
		m.produced.Add(1)
		yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("stream")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 7, OutputTokens: 2}}}, nil)
	}
}

func TestInstalledModelObserverConsumerStopJoinsOriginalProducer(t *testing.T) {
	observer, bindings, _ := installedObserverFixture(t, "transparent")
	model := &observerStreamModel{done: make(chan struct{})}
	_, streamer := observer.Bind(model, model, bindings)
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "stopped-stream", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "stopped-stream"}})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	for _, err := range streamer.Stream(ctx, request) {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	select {
	case <-model.done:
	default:
		t.Fatal("consumer stop detached the captured producer")
	}
	if model.produced.Load() != 1 {
		t.Fatal("consumer stop admitted another provider chunk")
	}
}

func installedObserverFixture(t *testing.T, mode string) (*ModelComposition, []plugins.ContributionBinding, *toolInstallationStore) {
	t.Helper()
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		t.Fatal(err)
	}
	name := testsupport.ServerName("observer")
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.observer", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "observer"}}, Contributions: []plugin.Contribution{{ID: "observer", Server: name, Export: "observer", Contract: contract.Identity()}}})
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
	remote := sdk.NewServer(&sdk.Implementation{Name: "installed-model-observer"}, nil)
	backend, err := pluginsdk.NewBackend(remote)
	if err != nil {
		t.Fatal(err)
	}
	call, _ := pluginsdk.ModelMethod()
	if err := call.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[chat.Request]) (pluginsdk.ModelResult, error) {
		next, err := pluginsdk.CallbackFromContext(ctx)
		if err != nil {
			return pluginsdk.ModelResult{}, err
		}
		request := command.Input.Clone()
		if mode == "transform" {
			request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
		}
		response, err := pluginsdk.CallModel(ctx, next, command.ID, request)
		if mode == "repeat" {
			_, _ = pluginsdk.CallModel(ctx, next, command.ID, request)
		}
		if mode == "outer-failure" {
			return pluginsdk.ModelResult{}, errors.New("observer failed after next")
		}
		if err != nil {
			return pluginsdk.ModelResult{}, err
		}
		return pluginsdk.ModelResult{Response: response}, nil
	}); err != nil {
		t.Fatal(err)
	}
	stream, _ := pluginsdk.ModelStreamMethod()
	if err := stream.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[chat.Request]) iter.Seq2[pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult], error] {
		return func(yield func(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult], error) bool) {
			next, err := pluginsdk.CallbackFromContext(ctx)
			if err != nil {
				yield(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult]{}, err)
				return
			}
			for value, err := range pluginsdk.StreamModel(ctx, next, command.ID, command.Input.Clone()) {
				if value.Result != nil && mode == "outer-failure" {
					yield(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult]{}, errors.New("observer failed after next"))
					return
				}
				if !yield(value, err) {
					return
				}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("observer", contract); err != nil {
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
	db, err := hostsqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compositions := hostsqlite.NewCompositionStore(db)
	choice, err := plugin.NewOrderedComposition(plugin.CompositionRecord{Contract: contract.Identity(), Members: []plugin.CompositionMember{{Installation: installation.ID(), Contribution: "observer"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := compositions.Save(t.Context(), choice); err != nil {
		t.Fatal(err)
	}
	routing, err := plugins.NewCommands(hostsqlite.NewPluginCommandStore(db), plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	calls, err := NewCommandCalls(store, hostsqlite.NewProviderSelectionStore(db), compositions, pool, routing, nil)
	if err != nil {
		t.Fatal(err)
	}
	observer, err := NewModelComposition(calls, callbacks, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := observer.Resolve(t.Context())
	if err != nil || len(bindings) != 1 {
		t.Fatalf("composition resolve: %+v, %v", bindings, err)
	}
	return observer, bindings, store
}
