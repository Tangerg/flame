package mcp

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	framevalue "github.com/Tangerg/frame/value"
	"golang.org/x/oauth2"
)

type memoryOAuthStore struct {
	mu      sync.Mutex
	target  framevalue.Digest
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

func mustHandshakeTimeout(t *testing.T, value time.Duration) mcpserver.HandshakeTimeout {
	t.Helper()
	policy, err := mcpserver.NewHandshakeTimeout(value)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}
