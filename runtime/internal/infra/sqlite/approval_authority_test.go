package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func openStore(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRemovedMCPSourceCannotRetainApproval(t *testing.T) {
	db := openStore(t)
	servers := sqlite.NewMCPServerStore(db)
	rules := sqlite.NewApprovalRuleStore(db)
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("github"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: "https://old.example/mcp"}
	if err := servers.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	ref := testsupport.MCPTool(server.ID(), "create_issue")
	rule, err := approval.NewRule(approval.ScopeGlobal, "", ref, server.AuthorityFingerprint(), approval.Subject{Type: approval.SubjectAll}, approval.Allow)
	if err != nil {
		t.Fatal(err)
	}
	if err := rules.Put(t.Context(), rule); err != nil {
		t.Fatal(err)
	}
	if err := servers.Remove(t.Context(), server.Name); err != nil {
		t.Fatal(err)
	}
	server.URL = "https://new.example/mcp"
	if err := servers.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	visible, err := rules.Visible(t.Context(), "", "", approval.MaximumVisibleRules)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("replacement server inherited approvals: %+v", visible)
	}
}

// Every relation about a user server references its source row, so one delete
// leaves nothing that a recreated server of the same name could inherit.
func TestUserServerRemovalCascadesEveryRelation(t *testing.T) {
	db := openStore(t)
	servers := sqlite.NewMCPServerStore(db)
	rules := sqlite.NewApprovalRuleStore(db)
	authorization := sqlite.NewMCPAuthorizationStore(db)
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("files"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: "https://files.example/mcp"}
	if err := servers.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	ref := testsupport.MCPTool(server.ID(), "write")
	rule, err := approval.NewRule(approval.ScopeGlobal, "", ref, server.AuthorityFingerprint(), approval.Subject{Type: approval.SubjectAll}, approval.Allow)
	if err != nil {
		t.Fatal(err)
	}
	if err := rules.Put(t.Context(), rule); err != nil {
		t.Fatal(err)
	}
	if err := servers.SetToolExposure(t.Context(), ref, true); err != nil {
		t.Fatal(err)
	}
	if _, err := authorization.BeginOAuthSession(t.Context(), server.OAuthTarget()); err != nil {
		t.Fatal(err)
	}
	requireRows(t, db, map[string]int{"mcp_sources": 1, "mcp_servers": 1, "approval_rules": 1, "mcp_tool_exposure": 1, "mcp_oauth_sessions": 1})

	if err := servers.Remove(t.Context(), server.Name); err != nil {
		t.Fatal(err)
	}
	requireRows(t, db, map[string]int{"mcp_sources": 0, "mcp_servers": 0, "approval_rules": 0, "mcp_tool_exposure": 0, "mcp_oauth_sessions": 0})
	if err := rules.Put(t.Context(), rule); err == nil {
		t.Fatal("a removed server regained a standing decision")
	}
	if err := servers.SetToolExposure(t.Context(), ref, true); err == nil {
		t.Fatal("a removed server regained an exposure decision")
	}
}

// A user server and an installation server may share a local name; each keeps
// its own relations, and removing one never touches the other.
func TestEqualNamesAcrossOriginsKeepSeparateRelations(t *testing.T) {
	db := openStore(t)
	servers := sqlite.NewMCPServerStore(db)
	rules := sqlite.NewApprovalRuleStore(db)
	installations := sqlite.NewInstallationStore(db)
	release := testsupport.Release(t, "release", plugin.Declaration{Name: "package", Servers: []plugin.Server{{Name: testsupport.ServerName("files"), Transport: mcpserver.TransportStdio, Command: "files"}}})
	if err := sqlite.NewReleaseStore(db).Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installations.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	installed, err := installation.ServerID(testsupport.ServerName("files"))
	if err != nil {
		t.Fatal(err)
	}
	user := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("files"), Enabled: true, Transport: mcpserver.TransportStdio, Command: "files"}
	if err := servers.Save(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	userRef := testsupport.MCPTool(user.ID(), "read")
	installedRef := testsupport.MCPTool(installed, "read")
	if userRef == installedRef || userRef.ModelName() != installedRef.ModelName() {
		t.Fatal("fixture must share a model name while differing in identity")
	}
	for _, ref := range []tool.Ref{userRef, installedRef} {
		rule, err := approval.NewRule(approval.ScopeGlobal, "", ref, testsupport.Digest("authority"), approval.Subject{Type: approval.SubjectAll}, approval.Deny)
		if err != nil {
			t.Fatal(err)
		}
		if err := rules.Put(t.Context(), rule); err != nil {
			t.Fatal(err)
		}
		if err := servers.SetToolExposure(t.Context(), ref, true); err != nil {
			t.Fatal(err)
		}
	}
	visible, err := rules.Visible(t.Context(), "", "", approval.MaximumVisibleRules)
	if err != nil || len(visible) != 2 || visible[0].Tool != userRef || visible[1].Tool != installedRef {
		t.Fatalf("rules = %+v, %v", visible, err)
	}

	if err := servers.Remove(t.Context(), user.Name); err != nil {
		t.Fatal(err)
	}
	visible, err = rules.Visible(t.Context(), "", "", approval.MaximumVisibleRules)
	if err != nil || len(visible) != 1 || visible[0].Tool != installedRef {
		t.Fatalf("removing the user server changed installation rules: %+v, %v", visible, err)
	}
	exposure, err := servers.ListExposure(t.Context())
	if err != nil || len(exposure) != 1 || exposure[0] != installedRef {
		t.Fatalf("removing the user server changed installation exposure: %+v, %v", exposure, err)
	}

	if err := installations.Remove(t.Context(), installation.Snapshot().ID); err != nil {
		t.Fatal(err)
	}
	requireRows(t, db, map[string]int{"mcp_sources": 0, "approval_rules": 0, "mcp_tool_exposure": 0})
}

func TestMCPSourceSchemaRejectsMixedOrigins(t *testing.T) {
	db := openStore(t)
	for _, statement := range []string{
		`INSERT INTO mcp_sources (origin, installation_id, name) VALUES ('user', 'eeb329cd-c7ce-40c9-bd90-6821fef06d30', 'files')`,
		`INSERT INTO mcp_sources (origin, name) VALUES ('installation', 'files')`,
		`INSERT INTO mcp_sources (origin, name) VALUES ('plugin', 'files')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement); err == nil {
			t.Errorf("accepted %s", statement)
		}
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO mcp_sources (origin, name) VALUES ('user', 'files')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO mcp_sources (origin, name) VALUES ('user', 'files')`); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate user source = %v", err)
	}
}

func requireRows(t *testing.T, db *sql.DB, want map[string]int) {
	t.Helper()
	for table, count := range want {
		var got int
		if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != count {
			t.Errorf("%s rows = %d, want %d", table, got, count)
		}
	}
}
