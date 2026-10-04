package sqlite_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestMCPServerStoreFencesOAuthCallbacksByGrant(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMCPServerStore(db)
	authorization := sqlite.NewMCPAuthorizationStore(db, store)
	server := mcpserver.Server{
		Name: testMCPServerName("remote"), Transport: mcpserver.TransportStreamableHTTP,
		Enabled: true, URL: "https://mcp.example.test/tools",
	}
	target := mcpserver.OAuthTarget{Server: server.Name, URL: server.URL}
	if err := store.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	origin := "https://mcp.example.test:443"
	oldBinding, err := authorization.BeginOAuthSession(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := authorization.LoadOAuthSession(t.Context(), target); err != nil || found {
		t.Fatalf("uncompleted authorization = found %v, err %v", found, err)
	}
	if err := authorization.SaveOAuthSession(t.Context(), server.Name, origin, oldBinding, []byte("old tokens")); err != nil {
		t.Fatal(err)
	}
	newBinding, err := authorization.BeginOAuthSession(t.Context(), target)
	if err != nil || newBinding == "" || newBinding == oldBinding {
		t.Fatalf("new authorization binding = %q, err %v", newBinding, err)
	}
	payload := []byte("current tokens")
	if err := authorization.SaveOAuthSession(t.Context(), server.Name, origin, newBinding, payload); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{oldBinding, ""} {
		if err := authorization.SaveOAuthSession(t.Context(), server.Name, origin, stale, []byte("late old refresh")); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
			t.Fatalf("stale refresh error = %v", err)
		}
		if err := authorization.RemoveOAuthSession(t.Context(), server.Name, stale); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
			t.Fatalf("stale rejection error = %v", err)
		}
	}
	if err := authorization.SaveOAuthSession(t.Context(), server.Name, "https://other.example:443", newBinding, payload); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
		t.Fatalf("cross-origin refresh error = %v", err)
	}
	got, binding, found, err := authorization.LoadOAuthSession(t.Context(), target)
	if err != nil || !found || binding != newBinding || !bytes.Equal(got, payload) {
		t.Fatalf("current credentials changed after stale callbacks: found=%v err=%v", found, err)
	}
	server.Description = "metadata update"
	if err := store.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	if _, binding, found, err := authorization.LoadOAuthSession(t.Context(), target); err != nil || !found || binding != newBinding {
		t.Fatalf("metadata edit revoked authorization: found=%v err=%v", found, err)
	}
	if err := authorization.RemoveOAuthSession(t.Context(), server.Name, newBinding); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := authorization.LoadOAuthSession(t.Context(), target); err != nil || found {
		t.Fatalf("removed credentials found=%v err=%v", found, err)
	}
}

func TestMCPServerStoreRevokesOAuthOnConfigurationChange(t *testing.T) {
	changes := []struct {
		name  string
		apply func(*mcpserver.Server)
	}{
		{name: "static authorization", apply: func(server *mcpserver.Server) { server.Authorization = "Bearer replacement" }},
		{name: "origin", apply: func(server *mcpserver.Server) { server.URL = "https://other.example.test/tools" }},
		{name: "path", apply: func(server *mcpserver.Server) { server.URL += "/new" }},
		{name: "headers", apply: func(server *mcpserver.Server) { server.Headers = map[string]string{"X-Tenant": "replacement"} }},
		{name: "disabled", apply: func(server *mcpserver.Server) { server.Enabled = false }},
		{name: "transport", apply: func(server *mcpserver.Server) {
			server.Transport, server.Command, server.URL = mcpserver.TransportStdio, "mcp-local", ""
		}},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			store := sqlite.NewMCPServerStore(db)
			authorization := sqlite.NewMCPAuthorizationStore(db, store)
			initial := mcpserver.Server{
				Name: testMCPServerName("remote"), Transport: mcpserver.TransportStreamableHTTP,
				Enabled: true, URL: "https://mcp.example.test/tools",
			}
			target := mcpserver.OAuthTarget{Server: initial.Name, URL: initial.URL}
			if err := store.Save(t.Context(), initial); err != nil {
				t.Fatal(err)
			}
			binding, err := authorization.BeginOAuthSession(t.Context(), target)
			if err != nil {
				t.Fatal(err)
			}
			changed := initial.Clone()
			change.apply(&changed)
			if err := store.Save(t.Context(), changed); err != nil {
				t.Fatal(err)
			}
			if _, err := authorization.BeginOAuthSession(t.Context(), target); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
				t.Fatalf("obsolete configuration obtained grant: %v", err)
			}
			if _, _, _, err := authorization.LoadOAuthSession(t.Context(), target); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
				t.Fatalf("obsolete configuration loaded credentials: %v", err)
			}
			if err := store.Save(t.Context(), initial); err != nil {
				t.Fatal(err)
			}
			if err := authorization.SaveOAuthSession(t.Context(), initial.Name, "https://mcp.example.test:443", binding, []byte("late old tokens")); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
				t.Fatalf("old grant resurrected after restoring configuration: %v", err)
			}
			if err := store.Remove(t.Context(), initial.Name); err != nil {
				t.Fatal(err)
			}
			if _, err := authorization.BeginOAuthSession(t.Context(), target); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
				t.Fatalf("deleted server obtained grant: %v", err)
			}
			if err := store.Save(t.Context(), initial); err != nil {
				t.Fatal(err)
			}
			newBinding, err := authorization.BeginOAuthSession(t.Context(), target)
			if err != nil || newBinding == binding {
				t.Fatalf("recreated server grant = %q, err %v", newBinding, err)
			}
			if err := authorization.RemoveOAuthSession(t.Context(), initial.Name, binding); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
				t.Fatalf("old rejection removed the recreated server's grant: %v", err)
			}
		})
	}
}

func TestMCPAuthorizationRefusesUnboundLegacyCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(t.Context(), `ALTER TABLE mcp_oauth_sessions DROP COLUMN target_fingerprint`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(t.Context(), path)
	if err == nil {
		_ = reopened.Close()
		t.Fatal("opened credentials without an authority binding")
	}
}
