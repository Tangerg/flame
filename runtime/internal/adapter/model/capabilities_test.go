package model

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/scope/models/catalog"

	modelsapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/llm"
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

func TestListModelsPreservesMissingCredentialCause(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		t.Error("model discovery reached the endpoint without a required credential")
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(server.Close)

	for _, providerID := range []string{"openai-compatible", "anthropic-compatible", "azureopenai"} {
		t.Run(providerID, func(t *testing.T) {
			entry := catalogProvider(t, providerID, "test-key", server.URL)
			entry, err := entry.Apply(provider.Patch{APIKey: provider.Clear[provider.APIKey]()})
			if err != nil {
				t.Fatal(err)
			}
			models, err := (Capabilities{}).ListModels(t.Context(), entry)
			if models != nil || !errors.Is(err, modelsapp.ErrProviderUnconfigured) || !errors.Is(err, ErrCredentialUnavailable) {
				t.Fatalf("ListModels = (%v, %v), want configuration error preserving its credential cause", models, err)
			}
		})
	}
}

func TestListModelsClassifiesRemoteFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`not-json secret-token`))
			}))
			t.Cleanup(server.Close)
			models, err := (Capabilities{}).ListModels(t.Context(), catalogProvider(t, "openai-compatible", "test-key", server.URL))
			if models != nil || !errors.Is(err, modelsapp.ErrModelDiscoveryFailed) {
				t.Fatalf("ListModels = (%v, %v), want discovery failure", models, err)
			}
		})
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

func TestProbeUsesAnthropicProtocolForCompatibleProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/models" {
			t.Errorf("request = %s %s, want GET /v1/models", request.Method, request.URL.Path)
		}
		if got := request.URL.Query().Get("limit"); got != "1000" {
			t.Errorf("limit = %q, want 1000", got)
		}
		if got := request.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want configured key", got)
		}
		if got := request.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q, want 2023-06-01", got)
		}
		if got := request.Header.Get("Authorization"); got != "" {
			t.Errorf("authorization = %q, want absent", got)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"id":"claude-compatible"}]}`))
	}))
	t.Cleanup(server.Close)

	err := (Capabilities{}).Probe(t.Context(), catalogProvider(t, "anthropic-compatible", "test-key", server.URL))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
}

func TestProbeRejectsPartialAnthropicModelCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"id":"first-page-model"}],"has_more":true,"last_id":"first-page-model"}`))
	}))
	t.Cleanup(server.Close)

	err := (Capabilities{}).Probe(t.Context(), catalogProvider(t, "anthropic-compatible", "test-key", server.URL))
	if err == nil {
		t.Fatal("Probe accepted a partial Anthropic model catalog")
	}
}

func TestProbeRejectsEndpointConfigurationChatAdapterCannotBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[{"id":"deployment"}]}`))
	}))
	t.Cleanup(server.Close)

	err := (Capabilities{}).Probe(t.Context(), catalogProvider(t, "azureopenai", "test-key", server.URL))
	if err == nil {
		t.Fatal("Probe accepted an Azure endpoint the chat adapter cannot build")
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

func TestProbeClassifiesRemoteRejections(t *testing.T) {
	for _, tt := range []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, modelsapp.ErrProviderCredentialsRejected},
		{http.StatusForbidden, modelsapp.ErrProviderCredentialsRejected},
		{http.StatusGatewayTimeout, context.DeadlineExceeded},
	} {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"error":{"message":"secret-token","type":"probe_error"}}`))
			}))
			t.Cleanup(server.Close)
			err := (Capabilities{}).Probe(t.Context(), catalogProvider(t, "openai-compatible", "test-key", server.URL))
			if !errors.Is(err, tt.want) {
				t.Fatalf("probe error = %v; want %v", err, tt.want)
			}
		})
	}
}
