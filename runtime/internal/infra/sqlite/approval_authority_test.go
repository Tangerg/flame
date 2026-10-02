package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestRemovedMCPSourceCannotRetainApproval(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	servers := sqlite.NewMCPServerStore(db)
	rules := sqlite.NewApprovalRuleStore(db)
	name, err := mcpserver.ParseServerName("github")
	if err != nil {
		t.Fatal(err)
	}
	server := mcpserver.Server{Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: "https://old.example/mcp"}
	if err := servers.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	remote, _ := mcpserver.ParseRemoteToolName("create_issue")
	ref, err := tool.MCP(name, remote)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := approval.NewRule(approval.ScopeGlobal, "", ref, server.AuthorityFingerprint(), approval.Subject{Type: approval.SubjectAll}, approval.Allow)
	if err != nil {
		t.Fatal(err)
	}
	if err := rules.Put(t.Context(), rule); err != nil {
		t.Fatal(err)
	}
	if err := servers.Remove(t.Context(), name); err != nil {
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
