package pluginservices

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

type routedCallResolver struct {
	model      chat.Model
	mu         sync.Mutex
	selections []modelref.Selection
}

func (r *routedCallResolver) ResolveChat(_ context.Context, selection modelref.Selection) (models.ResolvedChat, error) {
	r.mu.Lock()
	r.selections = append(r.selections, selection)
	r.mu.Unlock()
	return models.NewResolvedChat(r.model, nil)
}
func selectedCallInput(model string) pluginsdk.ModelCallRequest {
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original input")))
	request.Options.Model = model
	request.Options.MaxOutputTokens = new(int64(64))
	request.Tools = []chat.ToolDefinition{{Name: "echo", Description: "Echo.", InputSchema: jsontext.Value(`{"type":"object"}`)}}
	return pluginsdk.ModelCallRequest{Selection: pluginsdk.ModelSelection{Provider: "anthropic", Model: model}, Budget: pluginsdk.ModelCallBudget{MaxRequestBytes: 4096, MaxOutputTokens: 64}, Request: *request}
}
func selectedCallRef(t *testing.T, composition *ModelComposition, resolver models.ChatModelResolver) (pluginsdk.CallbackRef, func()) {
	t.Helper()
	callbacks, err := composition.ModelCallbacks(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if err := composition.callbacks.PublishRPC(http.NotFoundHandler(), callbacks...); err != nil {
		t.Fatal(err)
	}
	session := new(sdk.ClientSession)
	composition.callbacks.AdmitSession(session)
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-plugin-operation", Origin: capability.Model, Parent: "original-effect", Target: capability.Target{Scope: "call", Reference: "original-plugin-operation"}})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := composition.callbacks.OpenRPC(ctx, session, pluginsdk.CallbackLimits{MaxRequestBytes: 64 << 10, MaxResponseBytes: 64 << 10})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return ref, release
}
func TestSelectedModelCallsHaveIndependentRoutesBudgetsAndDurableEvidence(t *testing.T) {
	for _, outerFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(outerFailure), func(t *testing.T) {
			mode := "transparent"
			if outerFailure {
				mode = "outer-failure"
			}
			composition, _, _ := installedObserverFixture(t, mode)
			path := filepath.Join(t.TempDir(), "model-calls.db")
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			var physical atomic.Int32
			endpoint := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				physical.Add(1)
				inv, err := pluginsdk.InvocationFromContext(ctx)
				if err != nil || inv.Parent != "original-plugin-operation" || inv.Origin != capability.Model || inv.ID != inv.Target.Reference || len(request.Tools) != 1 {
					return nil, errors.New("independent model lost original caller or complete request")
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("known " + request.Options.Model))
				return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-" + request.Options.Model, Usage: &chat.Usage{InputTokens: 11, OutputTokens: 3}}}, nil
			})
			routing := &routedCallResolver{model: endpoint}
			observed, err := models.NewObservedResolver(routing, sqlite.NewModelInvocationStore(db), func(_ string, _ string, _ *chat.Usage) accounting.Cost {
				value, _ := accounting.NewCost(1.5)
				return value
			})
			if err != nil {
				t.Fatal(err)
			}
			ref, release := selectedCallRef(t, composition, observed)
			input := selectedCallInput("route-a")
			original := input.Request.Clone()
			for i := range 3 {
				input.Selection.Model = fmt.Sprintf("route-%d", i)
				input.Request.Options.Model = input.Selection.Model
				response, err := pluginsdk.CallSelectedModel(t.Context(), ref, fmt.Sprintf("independent-%d", i), input)
				if response == nil || response.Text() != "known "+input.Selection.Model || (err != nil) != outerFailure {
					t.Fatalf("independent outcome %d: %+v %v", i, response, err)
				}
			}
			input.Request.Options.Model = original.Options.Model
			if !reflect.DeepEqual(&input.Request, original) {
				t.Fatal("independent route mutated caller request")
			}
			input.Selection.Model = "route-0"
			input.Request.Options.Model = "route-0"
			if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "independent-0", input); err == nil || physical.Load() != 3 {
				t.Fatalf("redelivered model executed: %d %v", physical.Load(), err)
			}
			input.Request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
			if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "independent-0", input); err == nil || physical.Load() != 3 {
				t.Fatalf("changed identity executed: %d %v", physical.Load(), err)
			}
			if len(composition.HeldDependencies()) != 0 {
				t.Fatal("finished model calls retained contributions")
			}
			release()
			if _, err := pluginsdk.CallSelectedModel(t.Context(), ref, "after-release", input); !errors.Is(err, pluginsdk.ErrStaleCall) {
				t.Fatalf("released operation remained callable: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := sqlite.NewModelInvocationStore(db)
			rows, err := store.PageEndpointInvocations(t.Context(), "original-plugin-operation", 0, "", 10)
			if err != nil || len(rows) != 3 {
				t.Fatalf("durable independent endpoints: %+v %v", rows, err)
			}
			for _, row := range rows {
				if row.Invocation.Caller.Parent != "original-plugin-operation" || row.Invocation.Caller.Origin != capability.Model || row.Invocation.Model == "" || row.Response.Metadata.Model != "served-"+row.Invocation.Model || row.Response.Metadata.Usage.InputTokens != 11 || row.CostUSD == nil || *row.CostUSD != 1.5 {
					t.Fatalf("independent evidence replaced: %+v", row)
				}
			}
		})
	}
}

func TestSelectedModelFinalRequestBudgetRefusesBeforeProviderDispatch(t *testing.T) {
	for _, mode := range []string{"success", "media", "initial-output", "initial-bytes", "final-bytes"} {
		t.Run(mode, func(t *testing.T) {
			contract, err := pluginsdk.ModelInputContract()
			if err != nil {
				t.Fatal(err)
			}
			composition, _ := installedModelCompositionFixture(t, contract, []string{"converter"}, func(backend *pluginsdk.Backend) {
				method, _ := pluginsdk.ModelInputMethod()
				err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ModelInputRequest]) (chat.Request, error) {
					request := command.Input.Request.Clone()
					request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("converted"))
					if mode == "final-bytes" {
						request.Messages[0] = chat.NewUserMessage(chat.NewTextPart(string(make([]byte, 4096))))
					}
					return *request, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			})
			var calls atomic.Int32
			routing := &routedCallResolver{model: chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				calls.Add(1)
				if request.Messages[0].Text() != "converted" || len(request.Tools) != 1 {
					t.Error("provider did not receive complete transformed request")
				}
				message := chat.NewAssistantMessage(chat.NewTextPart("known"))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
			})}
			ref, _ := selectedCallRef(t, composition, routing)
			input := selectedCallInput("configured-model")
			if mode == "media" {
				image, err := media.NewBytes("image/png", []byte("source image"))
				if err != nil {
					t.Fatal(err)
				}
				input.Request.Messages[0] = chat.NewUserMessage(chat.NewMediaPart(image))
				input.Selection = pluginsdk.ModelSelection{Provider: "alibaba", Model: "qwen-mt-plus"}
				input.Request.Options.Model = input.Selection.Model
			}
			if mode == "initial-output" {
				input.Budget.MaxOutputTokens = 63
			}
			if mode == "initial-bytes" {
				input.Budget.MaxRequestBytes = 1
			}
			response, err := pluginsdk.CallSelectedModel(t.Context(), ref, "final-budget", input)
			if mode == "success" || mode == "media" {
				if err != nil || response == nil || calls.Load() != 1 {
					t.Fatalf("admitted request: %+v %d %v", response, calls.Load(), err)
				}
			} else if err == nil || calls.Load() != 0 {
				t.Fatalf("budget reached provider: %d %v", calls.Load(), err)
			}
			if len(composition.HeldDependencies()) != 0 {
				t.Fatal("refused model retained dependency")
			}
		})
	}
}

type selectedStreamModel struct {
	calls  atomic.Int32
	joined chan struct{}
}

func (m *selectedStreamModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, errors.New("unary path used for stream")
}
func (m *selectedStreamModel) Stream(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		m.calls.Add(1)
		defer close(m.joined)
		if !yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("first ")}, Metadata: &chat.ResponseMetadata{Model: "served-stream", Usage: &chat.Usage{InputTokens: 11, OutputTokens: 1}}}, nil) {
			return
		}
		if ctx.Err() != nil {
			yield(nil, ctx.Err())
			return
		}
		yield(&chat.ResponseDelta{Parts: []chat.PartDelta{chat.NewTextDelta("second")}, FinishReason: chat.FinishReasonStop, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 11, OutputTokens: 2}}}, nil)
	}
}
func TestSelectedModelStreamIsLazyPreservesEvidenceAndJoinsOnRelease(t *testing.T) {
	for _, stopEarly := range []bool{false, true} {
		t.Run(fmt.Sprint(stopEarly), func(t *testing.T) {
			composition, _, _ := installedObserverFixture(t, "transparent")
			db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "stream.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			endpoint := &selectedStreamModel{joined: make(chan struct{})}
			observed, err := models.NewObservedResolver(&routedCallResolver{model: endpoint}, sqlite.NewModelInvocationStore(db), nil)
			if err != nil {
				t.Fatal(err)
			}
			ref, release := selectedCallRef(t, composition, observed)
			input := selectedCallInput("stream-model")
			sequence := pluginsdk.StreamSelectedModel(t.Context(), ref, "independent-stream", input)
			input.Request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
			if endpoint.calls.Load() != 0 {
				t.Fatal("unconsumed stream started provider")
			}
			var result *pluginsdk.ModelResult
			for event, err := range sequence {
				if err != nil {
					t.Fatal(err)
				}
				if stopEarly {
					break
				}
				if event.Result != nil {
					result = event.Result
				}
			}
			release()
			<-endpoint.joined
			if endpoint.calls.Load() != 1 || len(composition.HeldDependencies()) != 0 {
				t.Fatal("stream source or dependency leaked")
			}
			if !stopEarly && (result == nil || result.Failure != nil || result.Response.Text() != "first second" || result.Response.Metadata.Usage.OutputTokens != 2) {
				t.Fatalf("canonical final outcome: %+v", result)
			}
			row, found, err := sqlite.NewModelInvocationStore(db).EndpointForCaller(t.Context(), "independent-stream")
			if err != nil || !found || row.Invocation.Caller.Parent != "original-plugin-operation" || row.Invocation.Caller.Origin != capability.Model || row.Invocation.Request.Messages[0].Text() != "original input" {
				t.Fatalf("stream original evidence: %+v %v", row, err)
			}
		})
	}
}

func TestSelectedModelResolvesInstalledSelectionForEveryIndependentCall(t *testing.T) {
	contract, err := pluginsdk.ModelSelectionContract()
	if err != nil {
		t.Fatal(err)
	}
	var choices atomic.Int32
	composition, _ := installedModelCompositionFixture(t, contract, []string{"selector"}, func(backend *pluginsdk.Backend) {
		method, _ := pluginsdk.ModelSelectionMethod()
		err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ModelSelectionRequest]) (pluginsdk.ModelSelection, error) {
			parent, err := pluginsdk.InvocationFromContext(ctx)
			if err != nil || parent.Parent != fmt.Sprintf("selected-route-%d", choices.Load()) || parent.Origin != capability.Model || command.Input.Request.Options.Model != "proposed-model" {
				return pluginsdk.ModelSelection{}, errors.New("selector lost original independent request")
			}
			chosen := choices.Add(1)
			return pluginsdk.ModelSelection{Provider: "anthropic", Model: fmt.Sprintf("chosen-%d", chosen)}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	routing := &routedCallResolver{model: chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		message := chat.NewAssistantMessage(chat.NewTextPart(request.Options.Model))
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, nil)
	})}
	ref, _ := selectedCallRef(t, composition, routing)
	input := selectedCallInput("proposed-model")
	for i := range 3 {
		response, err := pluginsdk.CallSelectedModel(t.Context(), ref, fmt.Sprintf("selected-route-%d", i), input)
		if err != nil || response == nil || response.Text() != fmt.Sprintf("chosen-%d", i+1) {
			t.Fatalf("fresh selection %d: %+v %v", i, response, err)
		}
	}
	if choices.Load() != 3 || input.Request.Options.Model != "proposed-model" {
		t.Fatal("independent selection reused capture or mutated caller")
	}
}
