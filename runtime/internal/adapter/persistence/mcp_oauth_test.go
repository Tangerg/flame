package persistence

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestMCPServerStoreFencesOAuthCallbacksByGrant(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	owner := testsupport.InstallationID(t)
	store := newMCPServerStore(db)
	authorization := newMCPAuthorizationStore(db)
	server := mcpserver.Server{
		Source:     mcpserver.UserSource(),
		Name:       testsupport.ServerName("remote"),
		Enabled:    true,
		Connection: mcpserver.HTTPConnection{URL: "https://mcp.example.test/tools"},
	}
	target, ok := server.OAuthTarget()
	if !ok {
		t.Fatal("HTTP server has no OAuth target")
	}
	if _, err := store.fixtureGrant(t.Context(), owner, server); err != nil {
		t.Fatal(err)
	}
	oldBinding, err := authorization.BeginOAuthSession(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := authorization.LoadOAuthSession(t.Context(), target); err != nil || found {
		t.Fatalf("uncompleted authorization = found %v, err %v", found, err)
	}
	if err := authorization.SaveOAuthSession(t.Context(), server.ID(), oldBinding, []byte("old tokens")); err != nil {
		t.Fatal(err)
	}
	newBinding, err := authorization.BeginOAuthSession(t.Context(), target)
	if err != nil || newBinding == "" || newBinding == oldBinding {
		t.Fatalf("new authorization binding = %q, err %v", newBinding, err)
	}
	payload := []byte("current tokens")
	if err := authorization.SaveOAuthSession(t.Context(), server.ID(), newBinding, payload); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{oldBinding, ""} {
		if err := authorization.SaveOAuthSession(t.Context(), server.ID(), stale, []byte("late old refresh")); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
			t.Fatalf("stale refresh error = %v", err)
		}
		if err := authorization.RemoveOAuthSession(t.Context(), server.ID(), stale); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
			t.Fatalf("stale rejection error = %v", err)
		}
	}
	got, binding, found, err := authorization.LoadOAuthSession(t.Context(), target)
	if err != nil || !found || binding != newBinding || !bytes.Equal(got, payload) {
		t.Fatalf("current credentials changed after stale callbacks: found=%v err=%v", found, err)
	}
	server.Description = "metadata update"
	if _, err := store.fixtureGrant(t.Context(), owner, server); err != nil {
		t.Fatal(err)
	}
	if _, binding, found, err := authorization.LoadOAuthSession(t.Context(), target); err != nil || !found || binding != newBinding {
		t.Fatalf("metadata edit revoked authorization: found=%v err=%v", found, err)
	}
	if err := authorization.RemoveOAuthSession(t.Context(), server.ID(), newBinding); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := authorization.LoadOAuthSession(t.Context(), target); err != nil || found {
		t.Fatalf("removed credentials found=%v err=%v", found, err)
	}
}

// A credential is bound to the target that requested it. Changing that target
// needs no invalidation step: the old credential simply stops matching, and
// removing the server removes it with its source.
func TestMCPCredentialsStopMatchingAChangedTarget(t *testing.T) {
	changes := []struct {
		name  string
		apply func(*mcpserver.Server)
	}{
		{name: "origin", apply: func(server *mcpserver.Server) {
			{
				updated := server.Connection.(mcpserver.HTTPConnection)
				updated.URL = "https://other.example.test/tools"
				server.Connection = updated
			}
		}},
		{name: "path", apply: func(server *mcpserver.Server) {
			{
				updated := server.Connection.(mcpserver.HTTPConnection)
				updated.URL += "/new"
				server.Connection = updated
			}
		}},
		{name: "headers", apply: func(server *mcpserver.Server) {
			{
				updated := server.Connection.(mcpserver.HTTPConnection)
				updated.Headers = map[string]string{"X-Tenant": "replacement"}
				server.Connection = updated
			}
		}},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			var triggers int
			if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND sql LIKE '%mcp_oauth_sessions%'`).Scan(&triggers); err != nil || triggers != 0 {
				t.Fatalf("credential invalidation triggers = %d, %v", triggers, err)
			}
			owner := testsupport.InstallationID(t)
			store := newMCPServerStore(db)
			authorization := newMCPAuthorizationStore(db)
			initial := mcpserver.Server{
				Source:     mcpserver.UserSource(),
				Name:       testsupport.ServerName("remote"),
				Enabled:    true,
				Connection: mcpserver.HTTPConnection{URL: "https://mcp.example.test/tools"},
			}
			if _, err := store.fixtureGrant(t.Context(), owner, initial); err != nil {
				t.Fatal(err)
			}
			binding, err := authorization.BeginOAuthSession(t.Context(), mustOAuthTarget(t, initial))
			if err != nil {
				t.Fatal(err)
			}
			if err := authorization.SaveOAuthSession(t.Context(), initial.ID(), binding, []byte("tokens")); err != nil {
				t.Fatal(err)
			}
			changed := initial.Clone()
			change.apply(&changed)
			if _, err := store.fixtureGrant(t.Context(), owner, changed); err != nil {
				t.Fatal(err)
			}
			if _, _, found, err := authorization.LoadOAuthSession(t.Context(), mustOAuthTarget(t, changed)); err != nil || found {
				t.Fatalf("changed target loaded the earlier credential: found=%v err=%v", found, err)
			}
			replacement, err := authorization.BeginOAuthSession(t.Context(), mustOAuthTarget(t, changed))
			if err != nil || replacement == binding {
				t.Fatalf("changed target grant = %q, err %v", replacement, err)
			}
			if err := authorization.SaveOAuthSession(t.Context(), initial.ID(), binding, []byte("late old tokens")); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
				t.Fatalf("superseded grant accepted a late refresh: %v", err)
			}
			if err := store.RetireGrants(t.Context(), owner, initial.Name, ""); err != nil {
				t.Fatal(err)
			}
			requireRows(t, db, map[string]int{"mcp_oauth_sessions": 0})
			if _, err := authorization.BeginOAuthSession(t.Context(), mustOAuthTarget(t, changed)); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
				t.Fatalf("deleted server obtained grant: %v", err)
			}
		})
	}
}

func TestMCPAuthorizationRefusesUnboundLegacyCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(t.Context(), `ALTER TABLE mcp_oauth_sessions DROP COLUMN target_fingerprint`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openDatabase(t.Context(), path)
	if err == nil {
		_ = reopened.Close()
		t.Fatal("opened credentials without an authority binding")
	}
}

func mustOAuthTarget(t *testing.T, server mcpserver.Server) mcpserver.OAuthTarget {
	t.Helper()
	target, ok := server.OAuthTarget()
	if !ok {
		t.Fatal("HTTP server has no OAuth target")
	}
	return target
}
