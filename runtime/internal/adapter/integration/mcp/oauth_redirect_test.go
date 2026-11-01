package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"golang.org/x/oauth2"
)

func TestRestoredRefreshDoesNotForwardTokenBody(t *testing.T) {
	target := mcpserver.OAuthTarget{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), URL: "https://mcp.example/api"}
	captured := make(chan struct{}, 1)
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { captured <- struct{}{}; w.WriteHeader(400) }))
	defer foreign.Close()
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL, http.StatusTemporaryRedirect)
	}))
	defer issuer.Close()
	token := &oauth2.Token{AccessToken: "expired", RefreshToken: "test-only", Expiry: time.Unix(1, 0)}
	cfg, _ := oauthSessionFixture(t, token)
	cfg.Endpoint.TokenURL = issuer.URL
	raw, err := encodeOAuthSession(cfg, token)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryOAuthStore{target: target.Fingerprint(), payload: raw}
	h, err := restoreOAuthHandler(t.Context(), t.Context(), store, target)
	if err != nil {
		t.Fatal(err)
	}
	src, err := h.TokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = src.Token(); err == nil {
		t.Fatal("accepted cross-origin token response")
	}
	select {
	case <-captured:
		t.Fatal("forwarded refresh POST")
	default:
	}
}
