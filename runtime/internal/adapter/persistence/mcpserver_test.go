package persistence

import (
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"path/filepath"
	"strings"
	"testing"
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
	if _, err := newMCPServerStore(db).Grant(t.Context(), testsupport.InstallationID(t), mcpserver.Server{Source: mcpserver.UserSource(), Name: server, Connection: mcpserver.StdioConnection{Command: "mcp-server"}}); err != nil {
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
	store := newMCPServerStore(db)
	owner := testsupport.InstallationID(t)
	s := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("local")}
	var old string
	for _, env := range []map[string]string{nil, {}, {"TOKEN": "value"}} {
		s.Connection = mcpserver.StdioConnection{Command: "server", Env: env}
		g, err := store.Grant(t.Context(), owner, s)
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
	s.Connection = mcpserver.HTTPConnection{URL: "file:///etc/passwd"}
	if _, err := store.Grant(t.Context(), owner, s); err == nil {
		t.Fatal("invalid endpoint admitted")
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE mcp_launch_grants SET material='{"http":{"URL":"file:///etc/passwd"}}'`); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.OpenGrant(t.Context(), owner, s.Name, old); ok || err == nil {
		t.Fatal("damaged material acquired")
	}
}
