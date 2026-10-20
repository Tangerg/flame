package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/plugins/web/backend/domain"
	"github.com/Tangerg/scope/tools/web"
	"github.com/Tangerg/scope/tools/web/jina"

	"github.com/Tangerg/flame/runtime/transport/httpredirect"
)

func TestProviderHTTPClientBoundsProviderResponseBeforeSDKDecode(t *testing.T) {
	body := `{"data":{"content":"` + strings.Repeat("x", int(maxProviderResponseBytes)) + `"}}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := jina.NewClient(jina.Config{
		APIKey: "test-key", FetchBaseURL: server.URL, HTTPClient: newProviderHTTPClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(t.Context(), &web.FetchRequest{URL: "https://example.test"})
	if !errors.Is(err, errProviderResponseTooLarge) {
		t.Fatalf("Fetch error = %v, want errProviderResponseTooLarge", err)
	}
}

func TestProviderFailureKeepsItsCauseWithoutPublishingCredentials(t *testing.T) {
	secret := "private-source-fixture-key"
	cause := errors.New("provider echoed " + secret)
	key, err := domain.NewAPIKey(&secret)
	if err != nil {
		t.Fatal(err)
	}
	source, err := jinaSource(key, &http.Client{Transport: sourceTransport(func(*http.Request) (*http.Response, error) { return nil, cause })})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source(t.Context(), &web.FetchRequest{URL: "https://example.test"})
	if !errors.Is(err, cause) || strings.Contains(err.Error(), secret) {
		t.Fatalf("provider failure lost cause or disclosed a key: %v", err)
	}
}

type sourceTransport func(*http.Request) (*http.Response, error)

func (f sourceTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestProviderHTTPClientBlocksCredentialRedirectAcrossOrigins(t *testing.T) {
	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHit.Store(true)
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("source request did not carry the provider credential")
		}
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)

	client, err := jina.NewClient(jina.Config{
		APIKey: "test-key", FetchBaseURL: source.URL, HTTPClient: newProviderHTTPClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(t.Context(), &web.FetchRequest{URL: "https://example.test"})
	if !errors.Is(err, httpredirect.ErrCrossOriginRedirect) {
		t.Fatalf("Fetch error = %v, want ErrCrossOriginRedirect", err)
	}
	if targetHit.Load() {
		t.Fatal("cross-origin online-provider redirect reached the target")
	}
}
