package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/httporigin"
)

func TestListRemoteModels(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"qwen2.5"},{"id":"llama3.2"},{"id":"llama3.2"}]}`))
	}))
	defer srv.Close()

	// A trailing slash on the base URL must not double up before /models.
	ids, err := listRemoteModels(t.Context(), srv.URL+"/v1/", "sk-test", modelListProtocolOpenAI)
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
		{name: "too long", id: strings.Repeat("m", modelref.MaximumModelIdentityCharacters+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(remoteModelList{Data: []remoteModel{{ID: tt.id}}})
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			if _, err := listRemoteModels(t.Context(), srv.URL, "", modelListProtocolOpenAI); err == nil {
				t.Fatalf("ListRemoteModels accepted invalid model identity %q", tt.id)
			}
		})
	}
}

func TestListRemoteModelsRejectsOversizedCompleteDocument(t *testing.T) {
	body := `{"data":[]}` + strings.Repeat(" ", maximumModelProbeResponseBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	if _, err := listRemoteModels(t.Context(), srv.URL, "", modelListProtocolOpenAI); err == nil {
		t.Fatal("ListRemoteModels accepted a document larger than its response envelope")
	}
}

func TestListRemoteModelsRejectsTrailingJSONValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}{"data":[{"id":"shadow"}]}`))
	}))
	defer srv.Close()

	if _, err := listRemoteModels(t.Context(), srv.URL, "", modelListProtocolOpenAI); err == nil {
		t.Fatal("ListRemoteModels accepted a second JSON document")
	}
}

func TestListRemoteModelsRejectsInvalidUTF8Document(t *testing.T) {
	body := append([]byte(`{"data":[{"id":"model-`), 0xff)
	body = append(body, []byte(`"}]}`)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	if _, err := listRemoteModels(t.Context(), srv.URL, "", modelListProtocolOpenAI); err == nil {
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
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	if _, err := listRemoteModels(t.Context(), srv.URL, "", modelListProtocolOpenAI); err == nil {
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
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	models, err := listRemoteModels(t.Context(), srv.URL, "", modelListProtocolOpenAI)
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
		_, _ = w.Write([]byte(exactBody))
	}))
	defer exactServer.Close()
	if _, err := listRemoteModels(t.Context(), exactServer.URL, "", modelListProtocolOpenAI); err != nil {
		t.Fatalf("ListRemoteModels rejected the exact byte boundary: %v", err)
	}
}

func TestListRemoteModelsNoKeyOmitsAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	if _, err := listRemoteModels(t.Context(), srv.URL, "", modelListProtocolOpenAI); err != nil {
		t.Fatalf("ListRemoteModels: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("auth header = %q, want none for a keyless local daemon", gotAuth)
	}
}

func TestListRemoteModelsNon200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := listRemoteModels(t.Context(), srv.URL, "", modelListProtocolOpenAI); err == nil {
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

	_, err := listRemoteModels(t.Context(), source.URL, "test-key", modelListProtocolAnthropic)
	if !errors.Is(err, httporigin.ErrCrossOriginRedirect) {
		t.Fatalf("ListRemoteModels error = %v, want ErrCrossOriginRedirect", err)
	}
	if targetHit.Load() {
		t.Fatal("cross-origin model probe redirect reached the target")
	}
}
