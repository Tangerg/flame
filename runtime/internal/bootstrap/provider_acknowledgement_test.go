package bootstrap

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestProviderConfigurationAndItsReceiptCommitTogether(t *testing.T) {
	const providerID = "openai-compatible"
	upstream := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(upstream.Close)
	cfg := poMCPConfig(t)
	cfg.modelProviders = nil
	host := poMCPOpen(t, cfg)
	activateModelProviders(host)
	update := func(path string) protocol.UpdateProviderRequest {
		return protocol.UpdateProviderRequest{Provider: providerID,
			BaseURL: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: new(upstream.URL + path)},
		}
	}
	endpoint := func() string {
		t.Helper()
		providers, err := host.api.ListProviders(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range providers.Data {
			if entry.ID == providerID && entry.BaseURL != nil {
				return *entry.BaseURL
			}
		}
		t.Fatalf("provider %q has no configured endpoint", providerID)
		return ""
	}
	host.must(delivery.ProvidersUpdate, update("/original"), "establish-provider-endpoint")
	clear := protocol.UpdateProviderRequest{Provider: providerID,
		BaseURL: &protocol.ProviderConfigChange{Type: protocol.ProviderConfigClear},
	}
	if result := host.endpoint.Invoke(t.Context(), delivery.ProvidersUpdate, clear, protocol.CallMetadata{IdempotencyKey: "clear-required-endpoint"}); !errors.Is(result.Failure, protocol.ErrInvalidParams) {
		t.Fatalf("clear the required endpoint = %v, want invalid_params", result.Failure)
	}
	if actual := endpoint(); actual != upstream.URL+"/original" {
		t.Fatalf("provider endpoint after rejected edit = %q, want the original", actual)
	}
	const key = "replace-provider-endpoint"
	request := update("/replacement")
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	refused := host.endpoint.Invoke(t.Context(), delivery.ProvidersUpdate, request, protocol.CallMetadata{IdempotencyKey: key})
	allowReceipt()
	if refused.Failure == nil {
		t.Fatal("replaced the endpoint despite a refused receipt")
	}
	if actual := endpoint(); actual != upstream.URL+"/original" {
		t.Fatalf("provider endpoint after a refused receipt = %q, want the original", actual)
	}
	host.must(delivery.ProvidersUpdate, request, key)
	host.must(delivery.ProvidersUpdate, update("/newer"), "change-provider-again")
	original := host.must(delivery.ProvidersUpdate, request, key).(*protocol.Provider)
	if current := endpoint(); original.BaseURL == nil || *original.BaseURL != upstream.URL+"/replacement" || current != upstream.URL+"/newer" {
		t.Fatalf("replay = %+v, standing provider endpoint = %q", original, current)
	}
}
