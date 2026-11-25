package bootstrap

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestProviderVerificationAcceptsAnEmptyAuthoritativeCatalog(t *testing.T) {
	const providerID = "openai-compatible"
	const apiKey = "empty-catalog-key"
	var modelReads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Error("the provider verification did not use its configured model endpoint and credential")
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		modelReads.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"object":"list","data":[]}`)
	}))
	t.Cleanup(upstream.Close)

	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	runtime := poMCPOpen(t, cfg)
	activateModelProviders(runtime)
	runtime.must(delivery.ProvidersUpdate, protocol.UpdateProviderRequest{
		Provider: providerID,
		APIKey:   &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: new(apiKey)},
		BaseURL:  &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: new(upstream.URL + "/v1")},
	}, "configure-empty-catalog")

	ctx := protocolLifecycleContext(t.Context())
	result, err := runtime.api.TestProvider(ctx, providerID)
	if err != nil || result.Outcome != protocol.ProviderTestReachable {
		t.Fatalf("provider verification = %+v, %v", result, err)
	}
	catalog, err := runtime.api.ListModels(ctx, protocol.ListModelsRequest{Provider: providerID})
	if err != nil || len(catalog.Data) != 0 || modelReads.Load() != 2 {
		t.Fatalf("provider catalog = %+v, %v, reads = %d", catalog, err, modelReads.Load())
	}
}
