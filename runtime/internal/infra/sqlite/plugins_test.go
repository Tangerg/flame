package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/google/uuid"
)

func TestInstallationTransitionsPreserveStandingRulesUntilSourceRemoval(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, rules := sqlite.NewInstallationStore(db), sqlite.NewApprovalRuleStore(db)
	release := plugin.Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []plugin.Server{{Name: "server", Type: "stdio", Command: "server"}}}
	if err := sqlite.NewReleaseStore(db).Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	i, err := plugin.New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release.Digest, nil); err != nil {
		t.Fatal(err)
	}
	if err := i.Enable(true); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	name, err := mcpserver.InstallationServer(i.Snapshot().ID, "server")
	if err != nil {
		t.Fatal(err)
	}
	remote, err := mcpserver.ParseRemoteToolName("write")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := tool.MCP(name, remote)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := approval.NewRule(approval.ScopeGlobal, "", ref, i.ServerAuthority("server"), approval.Subject{Type: approval.SubjectAll}, approval.Deny)
	if err != nil {
		t.Fatal(err)
	}
	if err := rules.Put(t.Context(), rule); err != nil {
		t.Fatal(err)
	}
	for _, transition := range []func() error{func() error { return i.Enable(false) }, func() error { return i.Enable(true) }, func() error { i.Revoke(); return nil }} {
		if err := transition(); err != nil {
			t.Fatal(err)
		}
		if err := store.Save(t.Context(), i); err != nil {
			t.Fatal(err)
		}
		visible, err := rules.Visible(t.Context(), "", "", approval.MaximumVisibleRules)
		if err != nil || len(visible) != 1 || visible[0].Decision != approval.Deny {
			t.Fatalf("standing deny lost: %+v, %v", visible, err)
		}
	}
	if err := store.Remove(t.Context(), i.Snapshot().ID); err != nil {
		t.Fatal(err)
	}
	visible, err := rules.Visible(t.Context(), "", "", approval.MaximumVisibleRules)
	if err != nil || len(visible) != 0 {
		t.Fatalf("removed source retained policy: %+v, %v", visible, err)
	}
	if err := rules.Put(t.Context(), rule); err == nil {
		t.Fatal("source integrity admitted a removed installation")
	}
	if _, err := store.Get(t.Context(), i.Snapshot().ID); !errors.Is(err, plugin.ErrNotFound) {
		t.Fatal(err)
	}
}

type oauthInstallationProjection struct{ current mcpserver.Server }

func (r *oauthInstallationProjection) Get(context.Context, mcpserver.ServerName) (mcpserver.Server, bool, error) {
	return r.current, true, nil
}

func TestInstallationCredentialRotationCannotResurrectAnOldGrant(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewInstallationStore(db)
	release := plugin.Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []plugin.Server{{Name: "server", Type: "streamable-http", URL: "https://mcp.example.test/tools"}}, Inputs: []plugin.Input{{ID: "tenant", Server: "server", Target: "header", Key: "X-Tenant", Secret: true}}}
	if err := sqlite.NewReleaseStore(db).Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	i, err := plugin.New(uuid.NewString(), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release.Digest, nil); err != nil {
		t.Fatal(err)
	}
	if err := i.Enable(true); err != nil {
		t.Fatal(err)
	}
	name, err := mcpserver.InstallationServer(i.Snapshot().ID, "server")
	if err != nil {
		t.Fatal(err)
	}
	registry := &oauthInstallationProjection{current: mcpserver.Server{Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: release.Servers[0].URL, ReleaseAuthority: i.ServerAuthority("server")}}
	authorization := sqlite.NewMCPAuthorizationStore(db, registry)
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	target := mcpserver.OAuthTarget{Server: name, URL: registry.current.URL, Authority: registry.current.ReleaseAuthority}
	binding, err := authorization.BeginOAuthSession(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorization.SaveOAuthSession(t.Context(), name, "https://mcp.example.test:443", binding, []byte("credential")); err != nil {
		t.Fatal(err)
	}
	tenant := "other"
	if err := i.Configure(plugin.Configuration{Digest: release.Digest, Values: map[string]*string{"tenant": &tenant}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	registry.current.Headers = map[string]string{"X-Tenant": tenant}
	if err := i.Configure(plugin.Configuration{Digest: release.Digest, Values: map[string]*string{"tenant": nil}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	registry.current.Headers = nil
	if err := authorization.SaveOAuthSession(t.Context(), name, "https://mcp.example.test:443", binding, []byte("late callback")); !errors.Is(err, mcpserver.ErrOAuthSessionSuperseded) {
		t.Fatalf("old grant revived: %v", err)
	}
	if _, _, found, err := authorization.LoadOAuthSession(t.Context(), target); err != nil || found {
		t.Fatalf("rotated credentials reused: %v, %v", found, err)
	}
}

func TestInstallationCannotAdvanceAnAdmittedDeclaration(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	catalog, store := sqlite.NewReleaseStore(db), sqlite.NewInstallationStore(db)
	release := plugin.Release{Digest: strings.Repeat("1", 64), Name: "package", Servers: []plugin.Server{{Name: "server", Type: plugin.Stdio, Command: "original"}}}
	if err := catalog.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	i, err := plugin.New(id, "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	release.Servers[0].Command = "replacement"
	changed, err := plugin.New(id, "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), changed); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Get(t.Context(), id)
	if err != nil || restored.Snapshot().Selected.Servers[0].Command != "original" {
		t.Fatalf("installation advanced declaration: %+v, %v", restored, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE plugin_releases SET declaration='{}' WHERE digest=?`, release.Digest); err == nil {
		t.Fatal("admitted declaration was mutable")
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE plugin_installations SET selected_digest=? WHERE id=?`, strings.Repeat("2", 64), id); err == nil {
		t.Fatal("installation selected an unadmitted release")
	}
	if err := store.Remove(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM mcp_sources`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("source survived uninstall: %d, %v", count, err)
	}
}
