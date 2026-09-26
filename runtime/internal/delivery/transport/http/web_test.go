package http_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/webassets"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestWebApplicationKeepsProtocolAuthenticationAndAssetConfinement(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"index.html":    "<!doctype html><title>flame browser</title>",
		"assets/app.js": "export const application = 'flame'",
		".private":      "must not be published",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("outside distribution"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := os.Symlink(external, filepath.Join(directory, "escape.txt")) == nil

	assets, err := webassets.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	server, err := flamehttp.NewServer(flamehttp.Config{
		Endpoint: newTestEndpoint(t, &fakeRuntime{}, delivery.EndpointConfig{IdempotencyNamespace: testsupport.IdempotencyNamespace}),
		Addr:     ":0", ProtocolVersion: protocol.ProtocolVersion,
		ServerInfo: protocol.ServerInfo{Name: "flame-test", Version: "dev", InstanceID: testRuntimeInstanceID},
		LocalToken: "web-test-token", WebApplication: assets,
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		method, path, accept, token string
		status                      int
		contains                    string
	}{
		{http.MethodGet, "/", "text/html", "", http.StatusOK, "flame browser"},
		{http.MethodGet, "/session/ses_browser", "text/html", "", http.StatusOK, "flame browser"},
		{http.MethodGet, "/assets/app.js", "*/*", "", http.StatusOK, "export const"},
		{http.MethodHead, "/assets/app.js", "*/*", "", http.StatusOK, ""},
		{http.MethodGet, "/assets/missing.js", "text/html", "", http.StatusNotFound, ""},
		{http.MethodGet, "/assets", "text/html", "", http.StatusNotFound, ""},
		{http.MethodGet, "/.private", "*/*", "", http.StatusNotFound, ""},
		{http.MethodGet, "/../external.txt", "*/*", "", http.StatusNotFound, ""},
		{http.MethodGet, "/..%5cexternal.txt", "*/*", "", http.StatusNotFound, ""},
		{http.MethodPost, "/", "*/*", "", http.StatusMethodNotAllowed, ""},
		{http.MethodGet, "/v2/info", "text/html", "", http.StatusOK, "protocolVersion"},
		{http.MethodPost, "/v2/rpc", "application/json", "", http.StatusUnauthorized, ""},
		{http.MethodGet, "/v2/missing", "text/html", "web-test-token", http.StatusNotFound, ""},
	}
	if linked {
		tests = append(tests, struct {
			method, path, accept, token string
			status                      int
			contains                    string
		}{http.MethodGet, "/escape.txt", "text/html", "", http.StatusNotFound, ""})
	}
	handler := server.Handler()
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("Accept", test.accept)
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("response = %d %q, want %d containing %q", response.Code, response.Body.String(), test.status, test.contains)
			}
			if strings.Contains(response.Body.String(), "outside distribution") || strings.Contains(response.Body.String(), "must not be published") {
				t.Fatal("web application escaped its public distribution")
			}
		})
	}
}
