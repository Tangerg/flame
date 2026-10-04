package pluginpackage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/google/uuid"
)

func BenchmarkInstalledSourceRuleList(b *testing.B) {
	source := b.TempDir()
	files := map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"remote":{"type":"streamable-http","url":"https://mcp.example/tools"}}}`}
	for index := range 4 {
		files[fmt.Sprintf("data-%d.txt", index)] = strings.Repeat("x", 1<<20)
	}
	for index := range 128 {
		files[fmt.Sprintf("note-%d.txt", index)] = "small resource"
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0600); err != nil {
			b.Fatal(err)
		}
	}
	db, err := sqlite.Open(b.Context(), filepath.Join(b.TempDir(), "flame.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	releases, err := New(b.TempDir(), sqlite.NewReleaseStore(db))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = filepath.WalkDir(releases.directory, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return err
		})
	})
	release, err := releases.Materialize(b.Context(), source)
	if err != nil {
		b.Fatal(err)
	}
	installation, err := plugin.New(uuid.NewString(), source, release)
	if err != nil {
		b.Fatal(err)
	}
	if err := installation.Approve(release.Digest, nil); err != nil {
		b.Fatal(err)
	}
	if err := installation.Enable(true); err != nil {
		b.Fatal(err)
	}
	installations := sqlite.NewInstallationStore(db)
	if err := installations.Save(b.Context(), installation); err != nil {
		b.Fatal(err)
	}
	registry, err := plugins.NewRegistry(sqlite.NewMCPServerStore(db), installations, releases)
	if err != nil {
		b.Fatal(err)
	}
	rules := sqlite.NewApprovalRuleStore(db)
	policy, err := approvals.NewRuntimePolicy(approval.ModeSafe, rules, sqlite.NewPermissionModeStore(db), toolset.NewAuthorities(registry.Definition, nil), nil)
	if err != nil {
		b.Fatal(err)
	}
	server, err := mcpserver.InstallationServer(installation.Snapshot().ID, "remote")
	if err != nil {
		b.Fatal(err)
	}
	for index := range 32 {
		remote, err := mcpserver.ParseRemoteToolName(fmt.Sprintf("read_%d", index))
		if err != nil {
			b.Fatal(err)
		}
		ref, err := tool.MCP(server, remote)
		if err != nil {
			b.Fatal(err)
		}
		if err := policy.SetRule(b.Context(), approvals.RuleChange{Tool: ref, Scope: approval.ScopeGlobal, Subject: approval.Subject{Type: approval.SubjectAll}, Decision: approval.Allow}, ""); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("rules", func(b *testing.B) {
		for range b.N {
			view, err := policy.Rules(b.Context(), "", "")
			if err != nil || len(view) != 32 {
				b.Fatalf("rules = %d, %v", len(view), err)
			}
		}
	})
	b.Run("storage", func(b *testing.B) {
		for range b.N {
			view, err := rules.Visible(b.Context(), "", "", approval.MaximumVisibleRules+1)
			if err != nil || len(view) != 32 {
				b.Fatalf("stored rules = %d, %v", len(view), err)
			}
		}
	})
}
