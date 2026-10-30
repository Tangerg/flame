package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestMCPServerStoreCannotPersistOrRestoreInvalidHTTPEndpoints(t *testing.T) {
	for _, endpoint := range []string{"/mcp", "file:///tmp/mcp", "https://", "https://user:secret@example.test/mcp"} {
		t.Run(endpoint, func(t *testing.T) {
			db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			store := newMCPServerStore(db)
			valid := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("remote"), Connection: mcpserver.HTTPConnection{URL: "https://example.test/mcp"}}
			if err := store.Save(t.Context(), valid); err != nil {
				t.Fatal(err)
			}
			invalid := valid.Clone()
			{
				updated := invalid.Connection.(mcpserver.HTTPConnection)
				updated.URL = endpoint
				invalid.Connection = updated
			}
			if err := store.Save(t.Context(), invalid); err == nil {
				t.Fatal("invalid endpoint persisted")
			}
			if stored, found, err := store.Get(t.Context(), valid.Name); err != nil || !found || stored.Connection.(mcpserver.HTTPConnection).URL != valid.Connection.(mcpserver.HTTPConnection).URL {
				t.Fatalf("rejected write changed the existing server: found=%t, err=%v", found, err)
			}
			if _, err := db.ExecContext(t.Context(), `UPDATE mcp_servers SET url=?`, endpoint); err != nil {
				t.Fatal(err)
			}
			if _, found, err := store.Get(t.Context(), valid.Name); err == nil || found {
				t.Fatal("invalid persisted endpoint became an MCP server")
			}
			if _, err := store.List(t.Context()); err == nil {
				t.Fatal("listing accepted an invalid persisted endpoint")
			}
		})
	}
}

func TestMCPServerStoreRetainsProcessEnvironmentPresence(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newMCPServerStore(db)
	for _, environment := range []map[string]string{nil, {}, {"TOKEN": "value"}} {
		server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("local"), Connection: mcpserver.StdioConnection{Command: "server", Env: environment}}
		if err := store.Save(t.Context(), server); err != nil {
			t.Fatal(err)
		}
		stored, found, err := store.Get(t.Context(), server.Name)
		if err != nil || !found || !server.SameConnection(stored) {
			t.Errorf("stored environment changed process inheritance: absent=%t, restoredAbsent=%t, found=%t, err=%v", environment == nil, stored.Connection.(mcpserver.StdioConnection).Env == nil, found, err)
		}
		listed, err := store.List(t.Context())
		if err != nil || len(listed) != 1 || !server.SameConnection(listed[0]) {
			t.Errorf("listed environment changed process inheritance: absent=%t, err=%v", environment == nil, err)
		}
	}
}

func TestMCPServerStoreRoundTrip(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newMCPServerStore(db)
	boundedTimeout, err := mcpserver.NewHandshakeTimeout(3 * time.Second)
	if err != nil {
		t.Fatalf("NewHandshakeTimeout: %v", err)
	}
	servers := []mcpserver.Server{
		{
			Source:           mcpserver.UserSource(),
			Name:             testsupport.ServerName("files"),
			Enabled:          true,
			Description:      "local files",
			HandshakeTimeout: boundedTimeout,
			Connection:       mcpserver.StdioConnection{Command: "mcp-files", Args: []string{"--root", "/repo"}, Env: map[string]string{"TOKEN": "secret"}, Dir: "/repo"},
		},
		{
			Source:     mcpserver.UserSource(),
			Name:       testsupport.ServerName("remote"),
			Enabled:    true,
			Connection: mcpserver.HTTPConnection{URL: "https://mcp.example.test", Authorization: "Bearer secret", Headers: map[string]string{"X-Trace": "enabled"}},
		},
	}
	for _, want := range servers {
		if err := want.Validate(); err != nil {
			t.Fatalf("invalid fixture %q: %v", want.Name, err)
		}
		if err := store.Save(t.Context(), want); err != nil {
			t.Fatalf("Save %q: %v", want.Name, err)
		}

		got, ok, err := store.Get(t.Context(), want.Name)
		if err != nil || !ok {
			t.Fatalf("Get %q: server=%+v ok=%v err=%v", want.Name, got, ok, err)
		}
		if !equalMCPServer(got, want) {
			t.Fatalf("Get %q round trip = %+v, want %+v", want.Name, got, want)
		}
	}
	listed, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != len(servers) {
		t.Fatalf("List count = %d, want %d", len(listed), len(servers))
	}
	wantByName := make(map[mcpserver.ServerName]mcpserver.Server, len(servers))
	for _, server := range servers {
		wantByName[server.Name] = server
	}
	for _, got := range listed {
		want, ok := wantByName[got.Name]
		if !ok || !equalMCPServer(got, want) {
			t.Fatalf("List contains %+v, want matching saved server", got)
		}
	}

	files := servers[0]
	files.Description = "updated description"
	if err := store.Save(t.Context(), files); err != nil {
		t.Fatalf("replace files description: %v", err)
	}
	replaced, found, err := store.Get(t.Context(), files.Name)
	if err != nil || !found || !equalMCPServer(replaced, files) {
		t.Fatalf("replaced files = %+v, found=%v err=%v", replaced, found, err)
	}
}

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
	if err := newMCPServerStore(db).Save(t.Context(), mcpserver.Server{Source: mcpserver.UserSource(), Name: server, Connection: mcpserver.StdioConnection{Command: "mcp-server"}}); err != nil {
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

func equalMCPServer(a, b mcpserver.Server) bool {
	return a.ID() == b.ID() && a.Enabled == b.Enabled && a.Description == b.Description &&
		a.HandshakeTimeout == b.HandshakeTimeout && a.SameConnection(b)
}

func TestMCPServerStoreRejectsMalformedJSONFields(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newMCPServerStore(db)
	server := mcpserver.Server{
		Source:     mcpserver.UserSource(),
		Name:       testsupport.ServerName("files"),
		Enabled:    true,
		Connection: mcpserver.StdioConnection{Command: "mcp-files"},
	}

	tests := []struct {
		name   string
		update string
		field  string
	}{
		{name: "headers", update: `UPDATE mcp_servers SET headers = '{' WHERE source_id = (SELECT id FROM mcp_sources WHERE origin = 'user' AND name = ?)`, field: "headers"},
		{name: "args", update: `UPDATE mcp_servers SET args = '{' WHERE source_id = (SELECT id FROM mcp_sources WHERE origin = 'user' AND name = ?)`, field: "args"},
		{name: "env", update: `UPDATE mcp_servers SET env = '{' WHERE source_id = (SELECT id FROM mcp_sources WHERE origin = 'user' AND name = ?)`, field: "env"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := store.Save(t.Context(), server); err != nil {
				t.Fatalf("Configure: %v", err)
			}
			if _, err := db.ExecContext(t.Context(), test.update, server.Name.String()); err != nil {
				t.Fatalf("corrupt %s: %v", test.field, err)
			}

			if _, ok, err := store.Get(t.Context(), server.Name); err == nil || ok ||
				!strings.Contains(err.Error(), `mcp server "files"`) || !strings.Contains(err.Error(), "decode "+test.field) {
				t.Fatalf("Get malformed %s: ok=%v err=%v", test.field, ok, err)
			}
			if _, err := store.List(t.Context()); err == nil ||
				!strings.Contains(err.Error(), `mcp server "files"`) || !strings.Contains(err.Error(), "decode "+test.field) {
				t.Fatalf("List malformed %s: err=%v", test.field, err)
			}
		})
	}
}

func TestMCPServerStorePreservesConfigurationWhenEncodingFails(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newMCPServerStore(db)
	original := mcpserver.Server{
		Source:     mcpserver.UserSource(),
		Name:       testsupport.ServerName("files"),
		Connection: mcpserver.StdioConnection{Command: "mcp-files", Args: []string{"--root", "/repo"}, Env: map[string]string{"MODE": "local"}},
	}
	if err := store.Save(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"args", "env"} {
		t.Run(field, func(t *testing.T) {
			invalid := original.Clone()
			if field == "args" {
				{
					updated := invalid.Connection.(mcpserver.StdioConnection)
					updated.Args = []string{"\xff"}
					invalid.Connection = updated
				}
			} else {
				{
					updated := invalid.Connection.(mcpserver.StdioConnection)
					updated.Env = map[string]string{"MODE": "\xff"}
					invalid.Connection = updated
				}
			}
			if err := store.Save(t.Context(), invalid); err == nil {
				t.Errorf("invalid UTF-8 in %s was silently saved as empty configuration", field)
			}
			got, found, err := store.Get(t.Context(), original.Name)
			if err != nil || !found || !equalMCPServer(got, original) {
				t.Fatalf("failed save changed server configuration: found=%v err=%v", found, err)
			}
		})
	}
}

func TestMCPServerStoreReadsOneConfigurationDuringConcurrentSave(t *testing.T) {
	for _, read := range []string{"get", "list"} {
		t.Run(read, func(t *testing.T) {
			db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			store := newMCPServerStore(db)
			initial := mcpserver.Server{
				Source:     mcpserver.UserSource(),
				Name:       testsupport.ServerName("files"),
				Connection: mcpserver.StdioConnection{Command: "first", Args: []string{"first"}},
			}
			replacement := initial.Clone()
			{
				updated := replacement.Connection.(mcpserver.StdioConnection)
				updated.Command = "second"
				replacement.Connection = updated
			}
			{
				updated := replacement.Connection.(mcpserver.StdioConnection)
				updated.Args = []string{"second"}
				replacement.Connection = updated
			}
			if err := store.Save(t.Context(), initial); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				for iteration := 0; iteration < 256; iteration++ {
					server := initial
					if iteration%2 == 0 {
						server = replacement
					}
					if err := store.Save(ctx, server); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()
			defer func() {
				cancel()
				if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
					t.Errorf("concurrent save: %v", err)
				}
			}()
			for range 256 {
				var servers []mcpserver.Server
				if read == "get" {
					server, found, err := store.Get(ctx, initial.Name)
					if err != nil || !found {
						t.Fatalf("Get: found=%v err=%v", found, err)
					}
					servers = []mcpserver.Server{server}
				} else {
					servers, err = store.List(ctx)
					if err != nil {
						t.Fatal(err)
					}
				}
				if len(servers) != 1 {
					t.Fatalf("server count = %d, want 1", len(servers))
				}
				server := servers[0]
				if !equalMCPServer(server, initial) && !equalMCPServer(server, replacement) {
					t.Fatalf("read combined one command with another command's arguments")
				}
			}
		})
	}
}
