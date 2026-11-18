package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	scopehttp "github.com/Tangerg/scope/core/httpclient"
)

func probeModels(t *testing.T, baseURL, apiKey string, provider Provider) ([]string, error) {
	t.Helper()
	return listModels(t.Context(), mustClientSpec(t, provider, apiKey, baseURL), nil)
}

func TestListRemoteModelsBlocksCredentialRedirectAcrossOrigins(t *testing.T) {
	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHit.Store(true)
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" || request.Header.Get("x-api-key") != "" {
			t.Error("source request did not carry the provider credential")
		}
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)

	_, err := probeModels(t, source.URL, "test-key", providerAnthropicCompatible)
	if !errors.Is(err, scopehttp.ErrOriginNotAllowed) {
		t.Fatalf("ListRemoteModels error = %v, want Scope origin refusal", err)
	}
	if targetHit.Load() {
		t.Fatal("cross-origin model probe redirect reached the target")
	}
}
