package main

import (
	"net/http"
	"net/http/httptest"

	"strings"
	"testing"
)

func TestStagedWebIndexIsServedAtTheNativeRoot(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	response := httptest.NewRecorder()
	options, err := desktopApplicationOptions(host)
	if err != nil {
		t.Fatal(err)
	}
	options.Assets.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	expected, err := assets.ReadFile("assets/web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	nonce := response.Header().Get("Content-Security-Policy")
	marker := "'nonce-"
	_, suffix, found := strings.Cut(nonce, marker)
	value, _, ended := strings.Cut(suffix, "'")
	if !found || !ended || value == "" {
		t.Fatal("native response has no script nonce")
	}
	expected = []byte(strings.ReplaceAll(string(expected), "__FLAME_RESPONSE_NONCE__", value))
	if response.Code != http.StatusOK || response.Body.String() != string(expected) {
		t.Fatalf("native root does not serve the staged Web index: status=%d", response.Code)
	}
}
