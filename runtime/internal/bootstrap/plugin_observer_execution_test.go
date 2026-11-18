package bootstrap

import (
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framevalue "github.com/Tangerg/frame/value"
)

func TestInstalledModelObserverPreservesExecutionEvidence(t *testing.T) {
	const (
		beforeCall     = "before"
		afterCall      = "after"
		providerAnswer = "Original Provider Answer"
		providerID     = "openai"
		modelID        = "gpt-5.6-sol"
	)
	type scenario struct {
		name         string
		params       *protocol.GenerationParams
		refusedPhase string
		lostResponse bool
		outcome      protocol.RunOutcomeType
		endpoint     protocol.ModelInvocationState
	}
	var active atomic.Pointer[scenario]
	active.Store(new(scenario))
	var generations atomic.Int32
	completion := fmt.Sprintf(`{"id":"original-response","model":%q,"status":"completed","output":[{"type":"message","id":"message","role":"assistant","content":[{"type":"output_text","text":%q}]}],"usage":{"input_tokens":7,"output_tokens":3}}`, modelID, providerAnswer)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/responses/input_tokens"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"object":"response.input_tokens","input_tokens":16}`)
		case strings.HasSuffix(request.URL.Path, "/responses"):
			generations.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			if active.Load().lostResponse {
				w.WriteHeader(http.StatusOK)
				return
			}
			delta := fmt.Sprintf(`{"type":"response.output_text.delta","item_id":"message","output_index":0,"content_index":0,"delta":%q}`, providerAnswer)
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", delta, completion)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(upstream.Close)

	unary, err := pluginsdk.ModelObserverCallMethod()
	if err != nil {
		t.Fatal(err)
	}
	unaryName := unary.Descriptor().Name
	var before, after, unaryObservations atomic.Int32
	trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var observed struct {
			Phase  string `json:"phase"`
			Method string `json:"method"`
		}
		if err := json.UnmarshalRead(request.Body, &observed); err != nil {
			t.Error(err)
			http.Error(w, "invalid trace", http.StatusBadRequest)
			return
		}
		switch observed.Phase {
		case beforeCall:
			before.Add(1)
			if observed.Method == unaryName {
				unaryObservations.Add(1)
			}
		case afterCall:
			after.Add(1)
		default:
			t.Errorf("unexpected observer phase %q", observed.Phase)
		}
		if observed.Phase == active.Load().refusedPhase {
			http.Error(w, "observation refused", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(trace.Close)

	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection(providerID, modelID)
	runtime := poMCPOpen(t, cfg)
	activateModelProviders(runtime)
	runtime.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{
		Provider: providerID,
		APIKey:   &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: new("test-key")},
		BaseURL:  &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &upstream.URL},
	}, "configure-provider")
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		t.Fatal(err)
	}
	observer := installCallContributor(t, runtime, buildCallContributor(t), "outer", trace.URL, contract)
	runtime.must(delivery.PluginsSetComposition, protocol.SetPluginCompositionRequest{
		Contract: contract.Identity().String(),
		Members:  []protocol.PluginCompositionMember{{InstallationID: observer.ID, Contribution: "contributor"}},
	}, "observe-model-call")

	for _, test := range []scenario{
		{name: "provider refused before dispatch", params: &protocol.GenerationParams{Stop: []string{"end"}}, outcome: protocol.OutcomeFailed},
		{name: "observer refused before continuation", refusedPhase: beforeCall, outcome: protocol.OutcomeFailed},
		{name: "generation observed", endpoint: protocol.ModelInvocationCompleted, outcome: protocol.OutcomeCompleted},
		{name: "completion acknowledgement failed", refusedPhase: afterCall, endpoint: protocol.ModelInvocationCompleted, outcome: protocol.OutcomeFailed},
		{name: "provider response and acknowledgement lost", refusedPhase: afterCall, lostResponse: true, endpoint: protocol.ModelInvocationUnknown, outcome: protocol.OutcomeFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			active.Store(&test)
			ctx := protocolLifecycleContext(t.Context())
			session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: test.name})
			if err != nil {
				t.Fatal(err)
			}
			started, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Answer the question."}},
				Params:    test.params,
			})
			if err != nil {
				t.Fatal(err)
			}
			waitForRunEvents(t, collectRunEvents(events), "observed model call to settle")
			settled, err := runtime.api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
			if err != nil || settled.Outcome == nil || settled.Outcome.Type != test.outcome {
				t.Fatalf("Run = %+v, %v, want %s", settled, err, test.outcome)
			}
			var wantSteps, wantAfter, wantUnresolved int
			if test.endpoint != "" {
				wantSteps = 1
			}
			if test.refusedPhase != beforeCall {
				wantAfter = 1
			}
			if test.endpoint == protocol.ModelInvocationUnknown {
				wantUnresolved = 1
			}
			if observedBefore, observedAfter, dispatched := before.Swap(0), after.Swap(0), generations.Swap(0); observedBefore != 1 || observedAfter != int32(wantAfter) || dispatched != int32(wantSteps) {
				t.Fatalf("observer before=%d after=%d, upstream generations=%d", observedBefore, observedAfter, dispatched)
			}
			if len(settled.Outcome.UnresolvedEffects) != wantUnresolved || settled.Metrics.Steps != wantSteps {
				t.Fatalf("observer changed execution evidence: %+v, metrics = %+v", settled.Outcome, settled.Metrics)
			}
			calls, err := runtime.api.ListModelInvocations(ctx, protocol.ListModelInvocationsRequest{RunID: started.RunID})
			if err != nil || len(calls.Data) != 1 {
				t.Fatalf("logical model calls = %+v, %v", calls, err)
			}
			endpoints, err := runtime.api.ListModelEndpoints(ctx, protocol.ListModelEndpointsRequest{ParentCallID: calls.Data[0].CallID})
			if err != nil || len(endpoints.Data) != wantSteps {
				t.Fatalf("physical provider calls = %+v, %v", endpoints, err)
			}
			if wantSteps == 1 && endpoints.Data[0].State != test.endpoint {
				t.Fatalf("physical provider call = %+v, want %s", endpoints.Data[0], test.endpoint)
			}
			if test.endpoint == protocol.ModelInvocationCompleted {
				if calls.Data[0].Usage == nil || calls.Data[0].Usage.InputTokens != 7 || calls.Data[0].Usage.OutputTokens != 3 {
					t.Fatalf("original provider usage was lost: %+v", calls.Data[0])
				}
				if test.refusedPhase == afterCall {
					known, knownErr := framevalue.ParseJSON([]byte(calls.Data[0].KnownResponse))
					original, originalErr := framevalue.ParseJSON([]byte(endpoints.Data[0].Response))
					if knownErr != nil || originalErr != nil || known != original {
						t.Fatalf("failed observer retained response %s (%v), want %s (%v)", known, knownErr, original, originalErr)
					}
				}
			}
		})
	}
	t.Run("unary title uses the captured response", func(t *testing.T) {
		active.Store(new(scenario))
		ctx := protocolLifecycleContext(t.Context())
		if _, err := runtime.api.SetUtilityRole(ctx, protocol.UtilityRole{Provider: providerID, Model: modelID}); err != nil {
			t.Fatal(err)
		}
		session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{})
		if err != nil {
			t.Fatal(err)
		}
		_, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
			SessionID: session.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Ask for a title."}},
		})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(events), "original model call to settle")
		if err := runtime.host.lifetime.runEffectTasks.Drain(ctx); err != nil {
			t.Fatal(err)
		}
		titled, err := runtime.api.GetSession(ctx, session.ID)
		if err != nil || titled.Title != providerAnswer {
			t.Fatalf("generated Session title = %+v, %v, want %q", titled, err, providerAnswer)
		}
		if unaryObservations.Load() != 1 || generations.Load() != 2 || before.Load() != 2 || after.Load() != 2 {
			t.Fatalf("observed title: unary observers=%d provider generations=%d before=%d after=%d", unaryObservations.Load(), generations.Load(), before.Load(), after.Load())
		}
	})
}
