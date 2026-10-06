package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/go-sdk/auth"
	"golang.org/x/oauth2"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func TestSupersededOAuthHandlersCannotChangeReplacementCredentials(t *testing.T) {
	for _, operation := range []string{"refresh", "refresh rejected", "server rejected"} {
		t.Run(operation, func(t *testing.T) {
			target := mcpserver.OAuthTarget{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), URL: "https://mcp.example/tools"}
			store := &memoryOAuthStore{}
			binding, err := store.BeginOAuthSession(t.Context(), target)
			if err != nil {
				t.Fatal(err)
			}
			old := &oauthSession{store: store, server: target.ID(), binding: binding}
			initial := &oauth2.Token{AccessToken: "initial", Expiry: time.Now().Add(time.Hour)}
			cfg, _ := oauthSessionFixture(t, initial)
			if err := old.save(t.Context(), cfg, initial); err != nil {
				t.Fatal(err)
			}

			replacementBinding, err := store.BeginOAuthSession(t.Context(), target)
			if err != nil {
				t.Fatal(err)
			}
			replacement := &oauthSession{store: store, server: target.ID(), binding: replacementBinding}
			if err := replacement.save(t.Context(), cfg, &oauth2.Token{AccessToken: "replacement"}); err != nil {
				t.Fatal(err)
			}
			before := bytes.Clone(store.payload)

			switch operation {
			case "refresh":
				source := newSavingTokenSource(tokenSourceFunc(func() (*oauth2.Token, error) {
					return &oauth2.Token{AccessToken: "late refresh"}, nil
				}), cfg, initial, func(cfg *oauth2.Config, token *oauth2.Token) error {
					return old.save(t.Context(), cfg, token)
				})
				_, err = source.Token()
			case "refresh rejected":
				source := invalidateRejectedTokens(tokenSourceFunc(func() (*oauth2.Token, error) {
					return nil, &oauth2.RetrieveError{ErrorCode: "invalid_grant"}
				}), t.Context(), old)
				_, err = source.Token()
			case "server rejected":
				handler := &restoredOAuthHandler{session: old}
				err = handler.Authorize(t.Context(), nil, &http.Response{Body: http.NoBody})
			}
			if !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
				t.Fatalf("stale %s did not retain the superseded grant error: %v", operation, err)
			}
			if !bytes.Equal(before, store.payload) || store.binding != replacementBinding {
				t.Fatal("superseded handler changed its replacement's credentials")
			}
		})
	}
}

type memoryOAuthStore struct {
	mu      sync.Mutex
	target  fingerprint.Digest
	payload []byte
	binding string
	removed int
	saveErr error
}

func (m *memoryOAuthStore) BeginOAuthSession(ctx context.Context, target mcpserver.OAuthTarget) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.target, m.binding, m.payload = target.Fingerprint(), rand.Text(), nil
	return m.binding, nil
}

func (m *memoryOAuthStore) LoadOAuthSession(_ context.Context, target mcpserver.OAuthTarget) ([]byte, string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.target != target.Fingerprint() || len(m.payload) == 0 {
		return nil, "", false, nil
	}
	return append([]byte(nil), m.payload...), m.binding, true, nil
}

func (m *memoryOAuthStore) SaveOAuthSession(ctx context.Context, _ mcpserver.ID, binding string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.saveErr != nil {
		return m.saveErr
	}
	if binding != m.binding {
		return mcpserver.ErrOAuthSessionSuperseded
	}
	m.payload = append([]byte(nil), payload...)
	return nil
}

func (m *memoryOAuthStore) RemoveOAuthSession(_ context.Context, _ mcpserver.ID, binding string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if binding != m.binding {
		return mcpserver.ErrOAuthSessionSuperseded
	}
	m.payload = nil
	m.removed++
	return nil
}

type tokenSourceFunc func() (*oauth2.Token, error)

func (t tokenSourceFunc) Token() (*oauth2.Token, error) { return t() }

type observedResponseBody struct {
	read   bool
	closed bool
}

func (b *observedResponseBody) Read([]byte) (int, error) {
	b.read = true
	return 0, errors.New("response body must not be read")
}

func (b *observedResponseBody) Close() error {
	b.closed = true
	return nil
}

func oauthSessionFixture(t *testing.T, token *oauth2.Token) (*oauth2.Config, []byte) {
	t.Helper()
	cfg := &oauth2.Config{
		ClientID: "client-id",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://auth.example/authorize",
			TokenURL: "https://auth.example/token",
		},
		RedirectURL: "http://127.0.0.1:3000/callback",
		Scopes:      []string{"tools.read"},
	}
	payload, err := encodeOAuthSession(cfg, token)
	if err != nil {
		t.Fatalf("encodeOAuthSession: %v", err)
	}
	return cfg, payload
}

func TestOAuthSessionRoundTripOwnsSlices(t *testing.T) {
	token := &oauth2.Token{
		AccessToken: "access", TokenType: "Bearer", RefreshToken: "refresh",
		Expiry: time.Now().Add(time.Hour).Round(time.Second),
	}
	cfg, payload := oauthSessionFixture(t, token)
	cfg.Scopes[0] = "mutated"

	gotConfig, gotToken, err := decodeOAuthSession(payload)
	if err != nil {
		t.Fatalf("decodeOAuthSession: %v", err)
	}
	// oauth2.Config and oauth2.Token are third-party structs with no redaction
	// boundary, so %+v of either prints a client secret and both tokens. Name
	// the field that disagreed instead.
	if gotConfig.Scopes[0] != "tools.read" {
		t.Fatalf("decoded scope = %q, want the unmutated fixture scope", gotConfig.Scopes[0])
	}
	if gotToken.AccessToken != token.AccessToken || gotToken.RefreshToken != token.RefreshToken {
		t.Fatal("decoded session did not round-trip its token material")
	}
	if !gotToken.Expiry.Equal(token.Expiry) {
		t.Fatalf("decoded expiry = %s, want %s", gotToken.Expiry, token.Expiry)
	}
}

func TestSavingTokenSourcePersistsOnlyChangedToken(t *testing.T) {
	initial := &oauth2.Token{AccessToken: "initial", Expiry: time.Now().Add(time.Minute)}
	refreshed := &oauth2.Token{AccessToken: "refreshed", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}
	cfg, _ := oauthSessionFixture(t, initial)
	var calls int
	source := newSavingTokenSource(tokenSourceFunc(func() (*oauth2.Token, error) {
		return refreshed, nil
	}), cfg, initial, func(*oauth2.Config, *oauth2.Token) error {
		calls++
		return nil
	})

	for range 2 {
		if token, err := source.Token(); err != nil || token.AccessToken != refreshed.AccessToken {
			t.Fatalf("Token = %+v, %v", token, err)
		}
	}
	if calls != 1 {
		t.Fatalf("save calls = %d, want 1", calls)
	}
}

func TestSavingTokenSourceFailsClosedWhenRefreshCannotPersist(t *testing.T) {
	initial := &oauth2.Token{AccessToken: "initial", Expiry: time.Now().Add(time.Minute)}
	refreshed := &oauth2.Token{AccessToken: "refreshed", Expiry: time.Now().Add(time.Hour)}
	cfg, _ := oauthSessionFixture(t, initial)
	wantErr := errors.New("disk unavailable")
	source := newSavingTokenSource(tokenSourceFunc(func() (*oauth2.Token, error) {
		return refreshed, nil
	}), cfg, initial, func(*oauth2.Config, *oauth2.Token) error { return wantErr })

	if token, err := source.Token(); token != nil || !errors.Is(err, wantErr) {
		t.Fatalf("Token = %+v, %v, want persistence error", token, err)
	}
}

func TestInvalidatingTokenSourceDeletesRejectedRefresh(t *testing.T) {
	store := &memoryOAuthStore{payload: []byte("saved")}
	source := invalidateRejectedTokens(tokenSourceFunc(func() (*oauth2.Token, error) {
		return nil, &oauth2.RetrieveError{ErrorCode: "invalid_grant"}
	}), t.Context(), &oauthSession{store: store, server: testsupport.UserMCPServer("remote"), binding: store.binding})

	if token, err := source.Token(); token != nil || !errors.Is(err, mcpserver.ErrAuthorizationRequired) {
		t.Fatalf("Token = %+v, %v, want needsAuth", token, err)
	}
	if store.removed != 1 || len(store.payload) != 0 {
		t.Fatalf("rejected refresh was not removed: %+v", store)
	}
	if _, err := source.Token(); !errors.Is(err, mcpserver.ErrAuthorizationRequired) || store.removed != 1 {
		t.Fatalf("repeated Token = %v, removed=%d", err, store.removed)
	}
}

func TestRestoreOAuthHandlerRejectsCredentialWithoutInteractiveFlow(t *testing.T) {
	token := &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}
	_, payload := oauthSessionFixture(t, token)
	target := mcpserver.OAuthTarget{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), URL: "https://MCP.example/tools"}
	store := &memoryOAuthStore{target: target.Fingerprint(), payload: payload}

	handler, err := restoreOAuthHandler(t.Context(), t.Context(), store, target)
	if err != nil {
		t.Fatalf("restoreOAuthHandler: %v", err)
	}
	if handler == nil {
		t.Fatal("restoreOAuthHandler returned nil")
	}
	source, err := handler.TokenSource(t.Context())
	if err != nil {
		t.Fatalf("TokenSource: %v", err)
	}
	if got, tokenErr := source.Token(); tokenErr != nil || got.AccessToken != token.AccessToken {
		t.Fatalf("restored token = %+v, %v", got, tokenErr)
	}

	response := &http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody}
	err = handler.Authorize(t.Context(), nil, response)
	if !errors.Is(err, errStoredOAuthRejected) {
		t.Fatalf("Authorize error = %v", err)
	}
	if source, err := handler.TokenSource(t.Context()); err != nil || source != nil {
		t.Fatalf("TokenSource after rejection = %v, %v", source, err)
	}
	if store.removed != 1 || len(store.payload) != 0 {
		t.Fatalf("rejected session was not removed: %+v", store)
	}
}

func TestRestoreOAuthHandlerDoesNotDrainRejectedResponse(t *testing.T) {
	store := &memoryOAuthStore{payload: []byte("saved")}
	body := &observedResponseBody{}
	handler := &restoredOAuthHandler{session: &oauthSession{store: store, server: testsupport.UserMCPServer("remote"), binding: store.binding}}

	err := handler.Authorize(t.Context(), nil, &http.Response{Body: body})
	if !errors.Is(err, errStoredOAuthRejected) {
		t.Fatalf("Authorize error = %v", err)
	}
	if body.read || !body.closed {
		t.Fatalf("response body read=%v closed=%v, want false, true", body.read, body.closed)
	}
	if store.removed != 1 {
		t.Fatalf("removed = %d, want 1", store.removed)
	}
}

func TestRestoreOAuthHandlerRejectsMalformedPayload(t *testing.T) {
	target := mcpserver.OAuthTarget{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), URL: "https://mcp.example/tools"}
	store := &memoryOAuthStore{target: target.Fingerprint(), payload: []byte(`{"unknown":true}`)}
	var handler auth.OAuthHandler
	handler, err := restoreOAuthHandler(t.Context(), t.Context(), store, target)
	if handler != nil || err == nil || !strings.Contains(err.Error(), "unknown object member name") {
		t.Fatalf("restore malformed = handler %v, err %v", handler, err)
	}
}
