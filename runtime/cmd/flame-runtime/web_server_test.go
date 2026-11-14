package main

import (
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestWebApplicationKeepsProtocolAuthenticationAndAssetConfinement(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"index.html":    `<!doctype html><title>flame browser</title><script nonce="TEST_NONCE" src="/assets/app.js"></script>`,
		"sandbox.html":  "<!doctype html><p>opaque wrapper</p>",
		"assets/app.js": "export const application = 'flame'",
		".private":      "must not be published",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	identities := map[string]any{}
	for _, name := range []string{"index.html", "sandbox.html", "assets/app.js"} {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		identity := map[string]string{"sha256": hex.EncodeToString(hash[:])}
		if name == "index.html" {
			identity["document"] = "main"
		}
		if name == "sandbox.html" {
			identity["document"] = "sandbox"
		}
		identities[name] = identity
	}
	policy, err := json.Marshal(map[string]any{
		"version": 1, "noncePlaceholder": "TEST_NONCE",
		"documentPolicy": "script-src 'nonce-TEST_NONCE' 'strict-dynamic'; frame-ancestors 'none'",
		"embeddedPolicy": "script-src 'nonce-TEST_NONCE' 'strict-dynamic'; frame-ancestors 'self'",
		"sandbox":        map[string]string{"file": "sandbox.html", "policy": "default-src 'none'"},
		"files":          identities,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, ".vite"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".vite", "flame-web-policy.json"), policy, 0o600); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("outside distribution"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := os.Symlink(external, filepath.Join(directory, "escape.txt")) == nil

	assets, err := newWebAssets(directory)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := delivery.NewEndpoint(struct{}{}, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: stores.Idempotency, IdempotencyNamespace: stores.IdempotencyNamespace.String()})
	if err != nil {
		t.Fatal(err)
	}
	server, err := flamehttp.NewServer(flamehttp.Config{
		Endpoint:       endpoint,
		Addr:           ":0",
		ServerInfo:     protocol.ServerInfo{Name: "flame-test", Version: "dev", InstanceID: testsupport.RuntimeInstanceID},
		Authentication: mustLocalToken(t, "web-test-token"), WebApplication: assets,
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
		{http.MethodGet, "/.vite/flame-web-policy.json", "*/*", "", http.StatusNotFound, ""},
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

func mustLocalToken(t testing.TB, token string) flamehttp.Authentication {
	t.Helper()
	authentication, err := flamehttp.LocalTokenAuthentication(token)
	if err != nil {
		t.Fatal(err)
	}
	return authentication
}
