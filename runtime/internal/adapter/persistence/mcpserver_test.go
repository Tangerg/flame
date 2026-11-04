package persistence

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestMCPServerSchemaRejectsNonCanonicalIdentity(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	invalid := []string{
		"",
		"GitHub",
		"with space",
		"server/name",
		strings.Repeat("a", mcpserver.MaximumServerNameCharacters+1),
	}
	for _, name := range invalid {
		if _, err := db.ExecContext(
			t.Context(),
			`INSERT INTO mcp_sources (origin, name) VALUES ('user', ?)`,
			name,
		); err == nil {
			t.Errorf("fresh schema accepted invalid MCP server identity %q", name)
		}
	}
}

func TestMCPServerExposureSchemaRejectsInvalidIdentityAndSource(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var source int64
	if err := db.QueryRowContext(
		t.Context(),
		`INSERT INTO mcp_sources (origin, name) VALUES ('user', 'files') RETURNING id`,
	).Scan(&source); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	invalidNames := []string{"", "with space", "tool/name", "工具", strings.Repeat("a", mcpserver.MaximumRemoteToolNameCharacters+1)}
	for _, toolName := range invalidNames {
		if _, err := db.ExecContext(
			t.Context(),
			`INSERT INTO mcp_tool_exposure (source_id, tool_name) VALUES (?, ?)`,
			source,
			toolName,
		); err == nil {
			t.Errorf("fresh schema accepted invalid remote tool identity %q", toolName)
		}
	}
	if _, err := db.ExecContext(
		t.Context(),
		`INSERT INTO mcp_tool_exposure (source_id, tool_name) VALUES (?, 'read')`,
		source+1,
	); err == nil {
		t.Error("fresh schema accepted an unknown MCP source")
	}
}

func TestMCPServerExposureSchemaEnforcesCardinality(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := testsupport.ServerName("files")
	if _, err := newMCPServerStore(databaseFor(db)).fixtureGrant(t.Context(), testsupport.InstallationID(t), mcpserver.Server{Source: mcpserver.UserSource(), Name: server, Connection: mcpserver.StdioConnection{Command: "mcp-server"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(
		t.Context(),
		`WITH RECURSIVE sequence(value) AS (
			SELECT 1
			UNION ALL
			SELECT value + 1 FROM sequence WHERE value < ?
		)
		INSERT INTO mcp_tool_exposure (source_id, tool_name)
		SELECT (SELECT id FROM mcp_sources WHERE origin = 'user' AND name = ?), 'tool_' || value FROM sequence`,
		mcpserver.MaxRemoteToolsPerServer,
		server.String(),
	); err != nil {
		t.Fatalf("insert maximum tool policy: %v", err)
	}
	if _, err := db.ExecContext(
		t.Context(),
		`INSERT INTO mcp_tool_exposure (source_id, tool_name) VALUES ((SELECT id FROM mcp_sources WHERE origin = 'user' AND name = ?), 'overflow')`,
		server.String(),
	); err == nil {
		t.Error("fresh schema accepted more than the MCP tool-policy cardinality limit")
	}
}

func TestMCPTransportGrantsAreBoundToConnectorServerAndExactMaterial(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := newMCPServerStore(databaseFor(db))
	owner := testsupport.InstallationID(t)
	s := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("local")}
	var old string
	for _, env := range []map[string]string{nil, {}, {"TOKEN": "value"}} {
		s.Connection = mcpserver.StdioConnection{Command: "server", Env: env}
		g, err := store.fixtureGrant(t.Context(), owner, s)
		if err != nil || g == old {
			t.Fatalf("grant did not distinguish launch material: %v", err)
		}
		conn, ok, err := store.OpenGrant(t.Context(), owner, s.Name, g)
		if err != nil || !ok {
			t.Fatal(ok, err)
		}
		current := s
		current.Connection = conn
		if !s.SameConnection(current) {
			t.Fatal("environment presence lost")
		}
		if _, ok, err := store.OpenGrant(t.Context(), testsupport.InstallationID(t), s.Name, g); ok || err != nil {
			t.Fatalf("unrelated connector acquired grant: %t, %v", ok, err)
		}
		if _, ok, err := store.OpenGrant(t.Context(), owner, testsupport.ServerName("other"), g); ok || err != nil {
			t.Fatalf("unrelated server acquired grant: %t, %v", ok, err)
		}
		if err := store.RetireGrants(t.Context(), owner, s.Name, g); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := store.OpenGrant(t.Context(), owner, s.Name, old); ok || err != nil {
			t.Fatal("previous grant remained current", err)
		}
		old = g
	}
	s.Connection = mcpserver.HTTPConnection{URL: "https://example.test/mcp", Authorization: "Bearer \xff", Headers: map[string]string{"X-Token": "\xfe"}}
	g, err := store.fixtureGrant(t.Context(), owner, s)
	if err != nil {
		t.Fatal(err)
	}
	restored, found, err := store.OpenGrant(t.Context(), owner, s.Name, g)
	if err != nil || !found || !s.SameConnection(mcpserver.Server{Source: s.Source, Name: s.Name, Connection: restored}) {
		t.Fatalf("HTTP grant failed byte-preserving round trip: found=%t, err=%v", found, err)
	}
	s.Connection = mcpserver.HTTPConnection{URL: "file:///etc/passwd"}
	if _, err := store.fixtureGrant(t.Context(), owner, s); err == nil {
		t.Fatal("invalid endpoint admitted")
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE mcp_launch_grants SET material='{"http":{"URL":"file:///etc/passwd"}}' WHERE grant_id=?`, old); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.OpenGrant(t.Context(), owner, s.Name, old); ok || err == nil {
		t.Fatal("damaged material acquired")
	}
}

func (m *MCPServerStore) fixtureGrant(ctx context.Context, owner identity.InstallationID, server mcpserver.Server) (string, error) {
	return m.storeGrant(ctx, owner, server, nil)
}

func TestRejectedGrantCanNeverExposeCandidateMaterial(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := newMCPServerStore(databaseFor(db))
	owner := testsupport.InstallationID(t)
	srv := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("local"), Connection: mcpserver.StdioConnection{Command: "original"}}
	current, err := s.fixtureGrant(t.Context(), owner, srv)
	if err != nil {
		t.Fatal(err)
	}
	proposed := srv.Clone()
	proposed.Connection = mcpserver.StdioConnection{Command: "refused"}
	token, err := s.storeGrant(t.Context(), owner, proposed, []byte("first candidate"))
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.OpenGrant(t.Context(), owner, srv.Name, token); found || err != nil {
		t.Fatalf("prepared grant open=%t err=%v", found, err)
	}
	if err := s.RejectGrant(t.Context(), owner, srv.Name, token); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.OpenGrant(t.Context(), owner, srv.Name, token); found || err != nil {
		t.Fatalf("rejected grant open=%t err=%v", found, err)
	}
	reused, err := s.storeGrant(t.Context(), owner, srv, []byte("same material"))
	if err != nil || reused != current {
		t.Fatal(reused, err)
	}
	if err := s.RejectGrant(t.Context(), owner, srv.Name, reused); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.OpenGrant(t.Context(), owner, srv.Name, current); !found || err != nil {
		t.Fatalf("active grant open=%t err=%v", found, err)
	}
	first, err := s.storeGrant(t.Context(), owner, proposed, []byte("lost acknowledgement"))
	if err != nil {
		t.Fatal(err)
	}
	newer := proposed.Clone()
	newer.Connection = mcpserver.StdioConnection{Command: "newer"}
	future, err := s.storeGrant(t.Context(), owner, newer, []byte("in progress"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmPendingGrant(t.Context(), owner, srv.Name, first); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.OpenGrant(t.Context(), owner, srv.Name, current); found || err != nil {
		t.Fatal("retired grant was still active", err)
	}
	if _, found, err := s.OpenGrant(t.Context(), owner, srv.Name, future); found || err != nil {
		t.Fatal("later command was prematurely active", err)
	}
	if err := s.RetireGrants(t.Context(), owner, srv.Name, future); err != nil {
		t.Fatalf("a late receipt revoked a later preparation: %v", err)
	}

}
