package bootstrap

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func activateModelProviders(r *poMCPRuntime) *protocol.PluginInstallation {
	r.t.Helper()
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "providers")}, "provider-install").(*protocol.PluginInstallation)
	installed = r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "provider-enable").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: "flame.model-provider.v1", InstallationID: installed.ID, Contribution: "model-provider"}, "select-model-provider")
	return installed
}

func TestInstalledModelProviderAbsenceAndDisablementLeaveManagementAvailable(t *testing.T) {
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	r := poMCPOpen(t, cfg)
	failed := r.endpoint.Invoke(t.Context(), delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "needs a provider"}, delivery.Options{IdempotencyKey: "without-provider", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
	if failed.Failure == nil || !errors.Is(failed.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("absent provider = %v", failed.Failure)
	}
	installed := activateModelProviders(r)
	r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "historical session"}, "with-provider")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable-provider")
	failed = r.endpoint.Invoke(t.Context(), delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "disabled provider"}, delivery.Options{IdempotencyKey: "without-enabled-provider", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
	cfg.provider, cfg.model = "openai-compatible", "frozen-model"
	cfg.approvalMode = approval.ModeYolo
	cfg.maintenance = noMaintenance{}
	r := poMCPOpen(t, cfg)
	installed := activateModelProviders(r)
	configure := func(endpoint, key, command string) {
		r.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{Provider: "openai-compatible", BaseURL: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &endpoint}, APIKey: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &key}}, command)
	}
	configure(original.URL, "original-key", "configure-original")
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "original provider"}, "create-original").(*protocol.Session)
	options := delivery.Options{IdempotencyKey: "start-original", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version, ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion}}}}
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
	blocked := r.endpoint.Invoke(t.Context(), delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: installed.ID}, delivery.Options{IdempotencyKey: "remove-captured-provider", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
