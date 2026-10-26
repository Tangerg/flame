package model

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	providerdomain "github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	scopehttp "github.com/Tangerg/scope/core/httpclient"
)

type remoteModelList struct {
	Data []remoteModel `json:"data"`
}

type remoteModel struct {
	ID string `json:"id"`
}

func probeModels(t *testing.T, baseURL, apiKey string, provider Provider) ([]string, error) {
	t.Helper()
	return listModels(t.Context(), mustClientSpec(t, provider, apiKey, baseURL))
}

func TestListRemoteModels(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"qwen2.5"},{"id":"llama3.2"},{"id":"llama3.2"}]}`))
	}))
	defer srv.Close()

	// A trailing slash on the base URL must not double up before /models.
	ids, err := probeModels(t, srv.URL+"/v1/", "sk-test", providerOpenAICompatible)
	if err != nil {
		t.Fatalf("ListRemoteModels: %v", err)
	}
	// Sorted and de-duplicated after every remote identity is admitted.
	if want := []string{"llama3.2", "qwen2.5"}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("probed path = %q, want /v1/models", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth header = %q, want Bearer sk-test", gotAuth)
	}
}

func TestListRemoteModelsRejectsInvalidAdvertisedIdentity(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "empty"},
		{name: "whitespace", id: "model shadow"},
		{name: "control", id: "model\nshadow"},
		{name: "too long", id: strings.Repeat("m", modelchoice.MaximumModelIdentityCharacters+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(remoteModelList{Data: []remoteModel{{ID: tt.id}}})
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			if _, err := probeModels(t, srv.URL, "test-key", providerOpenAICompatible); err == nil {
				t.Fatalf("ListRemoteModels accepted invalid model identity %q", tt.id)
			}
		})
	}
}

func TestListRemoteModelsRejectsOversizedCompleteDocument(t *testing.T) {
	body := `{"data":[]}` + strings.Repeat(" ", maximumModelProbeResponseBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	if _, err := probeModels(t, srv.URL, "test-key", providerOpenAICompatible); err == nil {
		t.Fatal("ListRemoteModels accepted a document larger than its response envelope")
	}
}

func TestListRemoteModelsRejectsTrailingJSONValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}{"data":[{"id":"shadow"}]}`))
	}))
	defer srv.Close()

	if _, err := probeModels(t, srv.URL, "test-key", providerOpenAICompatible); err == nil {
		t.Fatal("ListRemoteModels accepted a second JSON document")
	}
}

func TestListRemoteModelsRejectsInvalidUTF8Document(t *testing.T) {
	body := append([]byte(`{"data":[{"id":"model-`), 0xff)
	body = append(body, []byte(`"}]}`)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	if _, err := probeModels(t, srv.URL, "test-key", providerOpenAICompatible); err == nil {
		t.Fatal("ListRemoteModels normalized an invalid UTF-8 response")
	}
}

func TestListRemoteModelsRejectsOverfullCatalog(t *testing.T) {
	var list remoteModelList
	for index := range maximumRemoteModelCount + 1 {
		list.Data = append(list.Data, remoteModel{ID: fmt.Sprintf("model-%d", index)})
	}
	body, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	if _, err := probeModels(t, srv.URL, "test-key", providerOpenAICompatible); err == nil {
		t.Fatalf("ListRemoteModels accepted more than %d advertised models", maximumRemoteModelCount)
	}
}

func TestListRemoteModelsAcceptsResourceBoundaries(t *testing.T) {
	var list remoteModelList
	for index := range maximumRemoteModelCount {
		list.Data = append(list.Data, remoteModel{ID: fmt.Sprintf("model-%04d", index)})
	}
	body, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	models, err := probeModels(t, srv.URL, "test-key", providerOpenAICompatible)
	if err != nil {
		t.Fatalf("ListRemoteModels rejected the catalog boundary: %v", err)
	}
	if len(models) != maximumRemoteModelCount {
		t.Fatalf("models = %d, want %d", len(models), maximumRemoteModelCount)
	}

	padding := maximumModelProbeResponseBytes - len(`{"data":[]}`)
	if padding < 0 {
		t.Fatal("empty response exceeds byte boundary")
	}
	exactBody := `{"data":[]}` + strings.Repeat(" ", padding)
	exactServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(exactBody))
	}))
	defer exactServer.Close()
	if _, err := probeModels(t, exactServer.URL, "test-key", providerOpenAICompatible); err != nil {
		t.Fatalf("ListRemoteModels rejected the exact byte boundary: %v", err)
	}
}

func TestListModelsRejectsMissingCredentialBeforeIO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("model discovery reached the endpoint without a credential")
	}))
	defer server.Close()
	spec := mustClientSpec(t, providerOpenAICompatible, "test-key", server.URL)
	spec.credential = providerdomain.APIKey{}
	if _, err := listModels(t.Context(), spec); !errors.Is(err, providerdomain.ErrAPIKeyRequired) {
		t.Fatalf("missing credential error = %v", err)
	}
}

func TestListRemoteModelsNon200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := probeModels(t, srv.URL, "test-key", providerOpenAICompatible); err == nil {
		t.Fatal("expected an error on a non-200 probe, got nil")
	}
}

func TestListRemoteModelsBlocksCredentialRedirectAcrossOrigins(t *testing.T) {
	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHit.Store(true)
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") != "test-key" {
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
