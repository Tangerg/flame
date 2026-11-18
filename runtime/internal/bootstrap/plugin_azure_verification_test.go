package bootstrap

import (
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestAzureProviderVerifiesTheResourceAndRunsTheSelectedDeployment(t *testing.T) {
	const providerID = "azureopenai"
	const validKey = "resource-test-key"
	const rejectedKey = "resource-rejected-key"
	const deployment = "team-chat"
	const maxTokens = int64(128)

	var modelReads, generations atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		credential := request.Header.Get("Authorization")
		switch request.Method + " " + request.URL.Path {
		case http.MethodGet + " /openai/v1/models":
			modelReads.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if credential == "Bearer "+rejectedKey {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"error":{"type":"invalid_request_error","code":"invalid_api_key","message":"rejected"}}`)
				return
			}
			if credential != "Bearer "+validKey {
				t.Error("credential probe did not use the captured API key")
			}
			_, _ = fmt.Fprint(w, `{"object":"list","data":[{"id":"resource-model","object":"model","created":1,"owned_by":"azure"}]}`)
		case http.MethodPost + " /openai/v1/chat/completions":
			generations.Add(1)
			var input struct {
				Model               string `json:"model"`
				MaxCompletionTokens int64  `json:"max_completion_tokens"`
				MaxTokens           *int64 `json:"max_tokens"`
			}
			if err := json.UnmarshalRead(request.Body, &input); err != nil || input.Model != deployment || input.MaxCompletionTokens != maxTokens || input.MaxTokens != nil || credential != "Bearer "+validKey {
				t.Errorf("unexpected Azure deployment request: %+v, %v", input, err)
				http.Error(w, "invalid deployment request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: "+`{"id":"azure-reply","model":"served-model","choices":[{"index":0,"delta":{"role":"assistant","content":"deployment reply"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`+"\n\ndata: [DONE]\n\n")
		default:
			t.Errorf("unexpected Azure request %s %s", request.Method, request.URL.Path)
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(upstream.Close)
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	cfg.defaultSelection = testsupport.MustModelSelection(providerID, deployment)
	cfg.maintenance = noMaintenance{}
	runtime := poMCPOpen(t, cfg)
	activateModelProviders(runtime)
	endpoint := upstream.URL + "/openai/v1"
	ctx := protocolLifecycleContext(t.Context())
	configure := func(key, command string) {
		runtime.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{
			Provider: providerID,
			APIKey:   &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &key},
			BaseURL:  &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &endpoint},
		}, command)
	}
	configure(validKey, "configure-valid")
	result, err := runtime.api.TestProvider(ctx, providerID)
	if err != nil || result.Outcome != protocol.ProviderTestReachable || modelReads.Load() != 1 || generations.Load() != 0 {
		t.Fatalf("Azure credential probe = %+v, %v, model reads = %d, generations = %d", result, err, modelReads.Load(), generations.Load())
	}
	catalog, err := runtime.api.ListModels(ctx, protocol.ListModelsRequest{Provider: providerID})
	if err != nil || len(catalog.Data) != 0 || modelReads.Load() != 1 {
		t.Fatalf("resource models affected deployment discovery: %+v, %v, upstream reads = %d", catalog, err, modelReads.Load())
	}

	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Azure deployment"})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Answer the question."}},
		Params:    &protocol.GenerationParams{MaxTokens: new(maxTokens)},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "Azure deployment reply")
	run, err := runtime.api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted || generations.Load() != 1 {
		t.Fatalf("Azure deployment Run = %+v, %v, generations = %d", run, err, generations.Load())
	}

	configure(rejectedKey, "configure-rejected")
	result, err = runtime.api.TestProvider(ctx, providerID)
	if err != nil || result.Outcome != protocol.ProviderTestInvalidCredentials || modelReads.Load() != 2 || generations.Load() != 1 {
		t.Fatalf("Azure credential rejection = %+v, %v, model reads = %d, generations = %d", result, err, modelReads.Load(), generations.Load())
	}
}
