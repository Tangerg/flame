package modelcatalog

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/scope/models/catalog"

	"github.com/Tangerg/flame/runtime/internal/domain/provider"
	"github.com/Tangerg/flame/runtime/internal/infra/llm"
)

func TestCatalogContainsProviderDefaults(t *testing.T) {
	for _, provider := range llm.SupportedProviders() {
		model, hasDefault := provider.DefaultChatModel()
		if !hasDefault {
			continue
		}
		if _, ok := catalog.Default.Lookup(string(provider.ID()), model); !ok {
			t.Errorf("catalog has no default model %q for provider %q", model, provider.ID())
		}
	}
}

func TestProbeUsesRemoteModelsForProviderWithoutCatalogDefault(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/models" {
			t.Errorf("request = %s %s, want GET /models", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q, want bearer key", got)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"id":"served-model"}]}`))
	}))
	t.Cleanup(server.Close)

	err := (Capabilities{}).Probe(t.Context(), catalogProvider(t, "openai-compatible", "test-key", server.URL))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one endpoint-owned model probe", requests)
	}
}

func TestProbeRejectsProviderWithoutCatalogOrAdvertisedModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(server.Close)

	err := (Capabilities{}).Probe(t.Context(), catalogProvider(t, "openai-compatible", "test-key", server.URL))
	if err == nil {
		t.Fatal("Probe accepted an endpoint that advertised no usable model")
	}
}

func TestProbeUsesCatalogEndpointWithoutInventingOllamaCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Errorf("authorization = %q, want absent", authorization)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"id":"local-model"}]}`))
	}))
	t.Cleanup(server.Close)
	entry, err := provider.New("ollama")
	if err != nil {
		t.Fatal(err)
	}
	profile, found := llm.LookupProvider(llm.ProviderOllama)
	if !found || profile.RequiresAPIKey() {
		t.Fatal("ollama profile does not publish optional authentication")
	}
	baseURL, err := provider.NewBaseURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	entry, err = entry.Apply(provider.Patch{BaseURL: provider.Set(baseURL)})
	if err != nil {
		t.Fatal(err)
	}
	models, err := remoteModelIDs(t.Context(), entry)
	if err != nil || len(models) != 1 || models[0] != "local-model" {
		t.Fatalf("remote models = %v, %v", models, err)
	}
}

func catalogProvider(t *testing.T, id, rawKey, rawBaseURL string) provider.Provider {
	t.Helper()
	entry, err := provider.New(id)
	if err != nil {
		t.Fatal(err)
	}
	key, err := provider.NewAPIKey(rawKey)
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := provider.NewBaseURL(rawBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	entry, err = entry.Apply(provider.Patch{APIKey: provider.Set(key), BaseURL: provider.Set(baseURL)})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}
