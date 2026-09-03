package sqlite_test

import (
	"bytes"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestMCPServerStoreRoundTrip(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMCPServerStore(db)
	boundedTimeout, err := mcpserver.NewHandshakeTimeout(3 * time.Second)
	if err != nil {
		t.Fatalf("NewHandshakeTimeout: %v", err)
	}
	servers := []mcpserver.Server{
		{
			Name:             testMCPServerName("files"),
			Transport:        mcpserver.TransportStdio,
			Enabled:          true,
			Description:      "local files",
			Command:          "mcp-files",
			Args:             []string{"--root", "/repo"},
			Env:              map[string]string{"TOKEN": "secret"},
			Dir:              "/repo",
			HandshakeTimeout: boundedTimeout,
			ToolPolicy:       testServerToolPolicy([]string{"remove"}, []string{"read"}),
		},
		{
			Name:          testMCPServerName("remote"),
			Transport:     mcpserver.TransportStreamableHTTP,
			Enabled:       true,
			URL:           "https://mcp.example.test",
			Authorization: "Bearer secret",
			Headers:       map[string]string{"X-Trace": "enabled"},
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
	files.ToolPolicy = testServerToolPolicy(nil, []string{"stat"})
	if err := store.Save(t.Context(), files); err != nil {
		t.Fatalf("replace files policy: %v", err)
	}
	replaced, found, err := store.Get(t.Context(), files.Name)
	if err != nil || !found || !equalMCPServer(replaced, files) {
		t.Fatalf("replaced files = %+v, found=%v err=%v", replaced, found, err)
	}
}

func TestMCPServerSchemaRejectsNonCanonicalIdentity(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
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
			`INSERT INTO mcp_servers (name, transport, command) VALUES (?, 'stdio', 'mcp-server')`,
			name,
		); err == nil {
			t.Errorf("fresh schema accepted invalid MCP server identity %q", name)
		}
	}
}

func TestMCPServerToolPolicySchemaRejectsInvalidIdentityAndDecision(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := testMCPServerName("files")
	if _, err := db.ExecContext(
		t.Context(),
		`INSERT INTO mcp_servers (name, transport, command) VALUES (?, 'stdio', 'mcp-server')`,
		server.String(),
	); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	invalidNames := []string{"", "with space", "tool/name", "工具", strings.Repeat("a", mcpserver.MaximumRemoteToolNameCharacters+1)}
	for _, toolName := range invalidNames {
		if _, err := db.ExecContext(
			t.Context(),
			`INSERT INTO mcp_server_tool_policies (server_name, tool_name, decision) VALUES (?, ?, ?)`,
			server.String(),
			toolName,
			string(mcpserver.ToolDisabled),
		); err == nil {
			t.Errorf("fresh schema accepted invalid remote tool identity %q", toolName)
		}
	}
	if _, err := db.ExecContext(
		t.Context(),
		`INSERT INTO mcp_server_tool_policies (server_name, tool_name, decision) VALUES (?, 'read', 'maybe')`,
		server.String(),
	); err == nil {
		t.Error("fresh schema accepted unknown MCP tool-policy decision")
	}
}

func TestMCPServerToolPolicySchemaEnforcesCardinality(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	server := testMCPServerName("files")
	if _, err := db.ExecContext(
		t.Context(),
		`INSERT INTO mcp_servers (name, transport, command) VALUES (?, 'stdio', 'mcp-server')`,
		server.String(),
	); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	if _, err := db.ExecContext(
		t.Context(),
		`WITH RECURSIVE sequence(value) AS (
			SELECT 1
			UNION ALL
			SELECT value + 1 FROM sequence WHERE value < ?
		)
		INSERT INTO mcp_server_tool_policies (server_name, tool_name, decision)
		SELECT ?, 'tool_' || value, ? FROM sequence`,
		mcpserver.MaxRemoteToolsPerServer,
		server.String(),
		string(mcpserver.ToolDisabled),
	); err != nil {
		t.Fatalf("insert maximum tool policy: %v", err)
	}
	if _, err := db.ExecContext(
		t.Context(),
		`INSERT INTO mcp_server_tool_policies (server_name, tool_name, decision) VALUES (?, 'overflow', ?)`,
		server.String(),
		string(mcpserver.ToolDisabled),
	); err == nil {
		t.Error("fresh schema accepted more than the MCP tool-policy cardinality limit")
	}
}

func equalMCPServer(a, b mcpserver.Server) bool {
	return a.Name == b.Name && a.Transport == b.Transport && a.Enabled == b.Enabled &&
		a.Description == b.Description && a.URL == b.URL && a.Authorization == b.Authorization &&
		maps.Equal(a.Headers, b.Headers) && a.Command == b.Command && slices.Equal(a.Args, b.Args) &&
		maps.Equal(a.Env, b.Env) && a.Dir == b.Dir && a.HandshakeTimeout == b.HandshakeTimeout &&
		slices.Equal(a.ToolPolicy.Rules(), b.ToolPolicy.Rules())
}

func TestMCPServerStoreRejectsMalformedJSONFields(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMCPServerStore(db)
	server := mcpserver.Server{
		Name:      testMCPServerName("files"),
		Transport: mcpserver.TransportStdio,
		Enabled:   true,
		Command:   "mcp-files",
	}

	tests := []struct {
		name   string
		update string
		field  string
	}{
		{name: "headers", update: `UPDATE mcp_servers SET headers = '{' WHERE name = ?`, field: "headers"},
		{name: "args", update: `UPDATE mcp_servers SET args = '{' WHERE name = ?`, field: "args"},
		{name: "env", update: `UPDATE mcp_servers SET env = '{' WHERE name = ?`, field: "env"},
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

func TestMCPServerStoreBindsOAuthSessionLifecycle(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMCPServerStore(db)
	server := mcpserver.Server{
		Name: testMCPServerName("remote"), Transport: mcpserver.TransportStreamableHTTP,
		Enabled: true, URL: "https://mcp.example.test/tools",
	}
	if saveErr := store.Save(t.Context(), server); saveErr != nil {
		t.Fatalf("Save server: %v", saveErr)
	}

	origin := "https://mcp.example.test:443"
	payload := []byte(`{"version":1}`)
	if saveOAuthSessionErr := store.SaveOAuthSession(t.Context(), server.Name, origin, payload); saveOAuthSessionErr != nil {
		t.Fatalf("SaveOAuthSession: %v", saveOAuthSessionErr)
	}
	got, found, err := store.LoadOAuthSession(t.Context(), server.Name, origin)
	if err != nil || !found || !bytes.Equal(got, payload) {
		t.Fatalf("LoadOAuthSession = %q, %v, %v", got, found, err)
	}
	if _, found, err := store.LoadOAuthSession(t.Context(), server.Name, "https://other.example:443"); err != nil || found {
		t.Fatalf("cross-origin LoadOAuthSession: found=%v err=%v", found, err)
	}

	server.Description = "same endpoint"
	if err := store.Save(t.Context(), server); err != nil {
		t.Fatalf("Save metadata update: %v", err)
	}
	if _, found, err := store.LoadOAuthSession(t.Context(), server.Name, origin); err != nil || !found {
		t.Fatalf("metadata update removed OAuth session: found=%v err=%v", found, err)
	}
	server.Authorization = "Bearer static"
	if err := store.Save(t.Context(), server); err != nil {
		t.Fatalf("Save static authorization: %v", err)
	}
	if _, found, err := store.LoadOAuthSession(t.Context(), server.Name, origin); err != nil || found {
		t.Fatalf("static authorization retained OAuth session: found=%v err=%v", found, err)
	}
	server.Authorization = ""
	if err := store.Save(t.Context(), server); err != nil {
		t.Fatalf("Clear static authorization: %v", err)
	}
	if err := store.SaveOAuthSession(t.Context(), server.Name, origin, payload); err != nil {
		t.Fatalf("SaveOAuthSession after static authorization: %v", err)
	}

	server.URL = "https://new.example.test/tools"
	if err := store.Save(t.Context(), server); err != nil {
		t.Fatalf("Save endpoint update: %v", err)
	}
	if _, found, err := store.LoadOAuthSession(t.Context(), server.Name, origin); err != nil || found {
		t.Fatalf("endpoint update retained OAuth session: found=%v err=%v", found, err)
	}

	newOrigin := "https://new.example.test:443"
	if err := store.SaveOAuthSession(t.Context(), server.Name, newOrigin, payload); err != nil {
		t.Fatalf("SaveOAuthSession after update: %v", err)
	}
	if err := store.Remove(t.Context(), server.Name); err != nil {
		t.Fatalf("Remove server: %v", err)
	}
	if _, found, err := store.LoadOAuthSession(t.Context(), server.Name, newOrigin); err != nil || found {
		t.Fatalf("server removal retained OAuth session: found=%v err=%v", found, err)
	}
}
