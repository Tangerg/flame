package bootstrap

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func activateModelProviders(r *poMCPRuntime) *protocol.PluginInstallation {
	r.t.Helper()
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "providers")}, "provider-install").(*protocol.PluginInstallation)
	installed = r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "provider-enable").(*protocol.PluginInstallation)
	for _, contribution := range installed.Selected.Contributions {
		for i, role := range contribution.SelectionRoles {
			r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: role, InstallationID: installed.ID, Contribution: contribution.ID}, fmt.Sprintf("select-model-provider-%s-%d", installed.ID, i))
		}
	}
	return installed
}

func TestInstalledModelProviderAbsenceAndDisablementLeaveManagementAvailable(t *testing.T) {
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	r := poMCPOpen(t, cfg)
	failed := r.endpoint.Invoke(t.Context(), delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "needs a provider"}, protocol.CallMetadata{IdempotencyKey: "without-provider", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if failed.Failure == nil || !errors.Is(failed.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("absent provider = %v", failed.Failure)
	}
	installed := activateModelProviders(r)
	r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "historical session"}, "with-provider")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-provider")
	failed = r.endpoint.Invoke(t.Context(), delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "disabled provider"}, protocol.CallMetadata{IdempotencyKey: "without-enabled-provider", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if failed.Failure == nil || !errors.Is(failed.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("disabled provider = %v", failed.Failure)
	}
	page := r.must(delivery.SessionsList, protocol.ListSessionsRequest{}, "").(*protocol.Page[protocol.Session])
	if len(page.Data) != 1 || page.Data[0].Title != "historical session" {
		t.Fatalf("provider withdrawal changed history: %+v", page.Data)
	}
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-provider-again")
	r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "enabled again"}, "with-reenabled-provider")
}

func TestInstalledModelProviderRestartsWaitingRunWithOriginalCredentialsAndEndpoint(t *testing.T) {
	var calls atomic.Int32
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer original-key" {
			t.Error("waiting model changed its credential snapshot")
		}
		if !strings.HasSuffix(request.URL.Path, "/chat/completions") {
			http.NotFound(w, request)
			return
		}
		var input struct {
			Model string `json:"model"`
		}
		if err := json.UnmarshalRead(request.Body, &input); err != nil || input.Model != "frozen-model" {
			t.Errorf("captured identity changed: %s %v", input.Model, err)
		}
		delta := map[string]any{"role": "assistant", "content": "finished with original provider"}
		finish := "stop"
		if calls.Add(1) == 1 {
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "original-question", "type": "function", "function": map[string]string{"name": "ask_user", "arguments": `{"questions":[{"question":"Continue with the original model?"}]}`}}}}
			finish = "tool_calls"
		}
		frame := map[string]any{"id": "original-response", "model": "served-frozen-model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}}
		encoded, err := json.Marshal(frame)
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
	}))
	t.Cleanup(original.Close)
	var replacementCalls atomic.Int32
	replacement := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		replacementCalls.Add(1)
		http.Error(w, "unexpected replacement provider", 500)
	}))
	t.Cleanup(replacement.Close)
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection("openai-compatible", "frozen-model")
	cfg.approvalMode = approval.ModeYolo
	cfg.maintenance = noMaintenance{}
	r := poMCPOpen(t, cfg)
	installed := activateModelProviders(r)
	configure := func(endpoint, key, command string) {
		r.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{Provider: "openai-compatible", BaseURL: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &endpoint}, APIKey: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &key}}, command)
	}
	configure(original.URL, "original-key", "configure-original")
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "original provider"}, "create-original").(*protocol.Session)
	options := protocol.CallMetadata{IdempotencyKey: "start-original", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version, ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion}}}}
	started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "ask before continuing"}}}, options)
	if started.Failure != nil {
		t.Fatal(started.Failure)
	}
	for _, err := range started.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	runID := started.Value.(*protocol.StartRunResponse).RunID
	waiting := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
	if waiting.Status != protocol.RunStatusWaiting || calls.Load() != 1 {
		t.Fatalf("run did not retain its original wait: %+v calls=%d", waiting, calls.Load())
	}
	blocked := r.endpoint.Invoke(t.Context(), delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, protocol.CallMetadata{IdempotencyKey: "remove-captured-provider", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if blocked.Failure == nil || !errors.Is(blocked.Failure, protocol.ErrPluginInUse) {
		t.Fatalf("waiting provider was not retained: %v", blocked.Failure)
	}
	configure(replacement.URL, "replacement-key", "change-current-provider")
	r = reopenPluginRuntime(t, r, &cfg)
	if _, err := r.host.application.delivery.Models.ListProviders(t.Context()); err != nil {
		t.Fatal(err)
	}
	options.IdempotencyKey = ""
	pendingResult := r.endpoint.Invoke(t.Context(), delivery.InterruptsList, protocol.ListInterruptsRequest{RootRunID: runID}, options)
	if pendingResult.Failure != nil {
		t.Fatal(pendingResult.Failure)
	}
	pending := pendingResult.Value.(*protocol.Page[protocol.PendingInterruptSet])
	if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("restored wait: %+v", pending)
	}
	options.IdempotencyKey = "resume-original"
	resumed := r.endpoint.Invoke(t.Context(), delivery.RunsResume, protocol.ResumeRunRequest{RunID: runID, Responses: []protocol.InterruptResponse{{ItemID: pending.Data[0].Interrupts[0].ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Yes"}}}}}}, options)
	if resumed.Failure != nil {
		t.Fatal(resumed.Failure)
	}
	for _, err := range resumed.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	finished := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
	if finished.Status != protocol.RunStatusFinished || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted || calls.Load() != 2 || replacementCalls.Load() != 0 {
		t.Fatalf("original provider was replaced or repeated: %+v original=%d replacement=%d", finished, calls.Load(), replacementCalls.Load())
	}
}

func TestSelectedModelAdaptersCoexistPerProvider(t *testing.T) {
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	r := poMCPOpen(t, cfg)
	first := activateModelProviders(r)
	other := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "providers")}, "other-install").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: other.ID, Enabled: true}, "other-enable")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: extension.ModelProviderRole("openai"), InstallationID: other.ID, Contribution: "model-provider"}, "other-openai")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: false}, "withdraw-first")
	result := r.must("providers.list", struct{}{}, "").(*protocol.Page[protocol.Provider])
	if len(result.Data) != 1 || result.Data[0].ID != "openai" {
		t.Fatalf("available providers after withdrawal = %+v", result.Data)
	}
}

func TestModelEndpointAccountingFollowsDispatchEvidence(t *testing.T) {
	var exchanges atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/responses/input_tokens"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"object":"response.input_tokens","input_tokens":16}`)
		case strings.HasSuffix(request.URL.Path, "/responses"):
			exchanges.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(upstream.Close)
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection("openai", "gpt-5.6-sol")
	cfg.maintenance = noMaintenance{}
	runtime := poMCPOpen(t, cfg)
	activateModelProviders(runtime)
	runtime.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{
		Provider: "openai",
		APIKey:   &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: new("test-key")},
		BaseURL:  &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &upstream.URL},
	}, "configure-provider")

	for _, test := range []struct {
		name      string
		params    *protocol.GenerationParams
		wantSteps int
	}{
		{name: "preparation refused", params: &protocol.GenerationParams{Stop: []string{"end"}}},
		{name: "dispatched without a response", wantSteps: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			wantDispatch := test.wantSteps != 0
			priorExchanges := exchanges.Load()
			session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: test.name})
			if err != nil {
				t.Fatal(err)
			}
			started, events, err := runtime.api.StartRun(protocolLifecycleContext(ctx), protocol.StartRunRequest{
				SessionID: session.ID,
				Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Answer the question."}},
				Params:    test.params,
			})
			if err != nil {
				t.Fatal(err)
			}
			waitForRunEvents(t, collectRunEvents(events), "model exchange to settle")
			settled, err := runtime.api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
			if err != nil || settled.Status != protocol.RunStatusFinished || settled.Outcome == nil {
				t.Fatalf("Run did not settle: %+v, %v", settled, err)
			}
			if !wantDispatch && settled.Outcome.Type != protocol.OutcomeFailed {
				t.Fatalf("preparation refusal outcome = %+v, want failed", settled.Outcome)
			}
			if settled.Metrics.Steps != test.wantSteps || (len(settled.Outcome.UnresolvedEffects) != 0) != wantDispatch {
				t.Fatalf("Run metrics = %+v, outcome = %+v", settled.Metrics, settled.Outcome)
			}
			calls, err := runtime.api.ListModelInvocations(ctx, protocol.ListModelInvocationsRequest{RunID: started.RunID})
			if err != nil || len(calls.Data) != 1 || calls.Data[0].State != protocol.ModelInvocationFailed {
				t.Fatalf("logical Model calls = %+v, %v, want one failed call", calls, err)
			}
			endpoints, err := runtime.api.ListModelEndpoints(ctx, protocol.ListModelEndpointsRequest{ParentCallID: calls.Data[0].CallID})
			if err != nil || len(endpoints.Data) != test.wantSteps {
				t.Fatalf("physical endpoints = %+v, %v, want %d", endpoints, err, test.wantSteps)
			}
			if wantDispatch && endpoints.Data[0].State != protocol.ModelInvocationUnknown {
				t.Fatalf("unconfirmed physical endpoint = %+v", endpoints.Data[0])
			}
			if dispatched := exchanges.Load() > priorExchanges; dispatched != wantDispatch {
				t.Fatalf("upstream received a generation request = %t, want %t", dispatched, wantDispatch)
			}
		})
	}
}
