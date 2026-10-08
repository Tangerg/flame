package sqlite_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestInstallationTransitionsPreserveStandingRulesUntilSourceRemoval(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, rules := sqlite.NewInstallationStore(db), sqlite.NewApprovalRuleStore(db)
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{{Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "server"}}})
	if err := sqlite.NewReleaseStore(db).Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	i, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := i.Enable(release); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	name, err := i.ServerID(testsupport.ServerName("server"))
	if err != nil {
		t.Fatal(err)
	}
	ref := testsupport.MCPTool(name, "write")
	if err := sqlite.NewMCPServerStore(db).SetToolExposure(t.Context(), ref, true); err != nil {
		t.Fatal(err)
	}
	rule, err := approval.NewRule(approval.ScopeGlobal, "", ref, i.ServerAuthority(testsupport.ServerName("server")), approval.Subject{Type: approval.SubjectAll}, approval.Deny)
	if err != nil {
		t.Fatal(err)
	}
	if err := rules.Put(t.Context(), rule); err != nil {
		t.Fatal(err)
	}
	for _, transition := range []func() error{func() error { i.Disable(); return nil }, func() error { return i.Enable(release) }, func() error { i.Revoke(); return nil }} {
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
	if err := store.Remove(t.Context(), i.ID()); err != nil {
		t.Fatal(err)
	}
	visible, err := rules.Visible(t.Context(), "", "", approval.MaximumVisibleRules)
	if err != nil || len(visible) != 0 {
		t.Fatalf("removed source retained policy: %+v, %v", visible, err)
	}
	requireRows(t, db, map[string]int{"mcp_sources": 0, "mcp_tool_exposure": 0})
	if err := rules.Put(t.Context(), rule); err == nil {
		t.Fatal("source integrity admitted a removed installation")
	}
	if _, err := store.Get(t.Context(), i.ID()); !errors.Is(err, plugin.ErrNotFound) {
		t.Fatal(err)
	}
}

// Installation changes never delete credentials. A credential stops matching
// once its recipient changes, and goes with its source.
func TestInstallationCredentialsFollowTheirRecipient(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, authorization, catalog := sqlite.NewInstallationStore(db), sqlite.NewMCPAuthorizationStore(db), sqlite.NewReleaseStore(db)
	server := testsupport.ServerName("server")
	declare := func(seed, url string) plugin.Release {
		release := testsupport.Release(t, seed, plugin.Declaration{Name: "package", Version: seed, Servers: []plugin.Server{{Name: server, Transport: mcpserver.TransportStreamableHTTP, URL: url}}, Inputs: []plugin.Input{{ID: "tenant", Server: server, Target: plugin.Header, Key: "X-Tenant", Secret: true}}})
		if err := catalog.Admit(t.Context(), release); err != nil {
			t.Fatal(err)
		}
		return release
	}
	release := declare("1", "https://mcp.example.test/tools")
	i, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	target := func(selected plugin.Release) mcpserver.OAuthTarget {
		source, err := i.ServerSource(selected, server)
		if err != nil {
			t.Fatal(err)
		}
		return mcpserver.OAuthTarget{Source: source, Name: server, URL: selected.Declaration().Servers[0].URL}
	}
	initial := target(release)
	binding, err := authorization.BeginOAuthSession(t.Context(), initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorization.SaveOAuthSession(t.Context(), initial.ID(), binding, []byte("credential")); err != nil {
		t.Fatal(err)
	}
	selectRelease := func(selected, candidate plugin.Release) {
		t.Helper()
		if err := i.Stage(selected, candidate); err != nil {
			t.Fatal(err)
		}
		if err := i.Select(selected, candidate); err != nil {
			t.Fatal(err)
		}
		if err := store.Save(t.Context(), i); err != nil {
			t.Fatal(err)
		}
	}
	if err := i.Configure(release, plugin.Configuration{Values: map[string]plugin.ValueChange{"tenant": plugin.SetValue("other")}}); err != nil {
		t.Fatal(err)
	}
	sameEndpoint := declare("2", "https://mcp.example.test/tools")
	selectRelease(release, sameEndpoint)
	if _, _, found, err := authorization.LoadOAuthSession(t.Context(), target(sameEndpoint)); err != nil || !found {
		t.Fatalf("an unchanged endpoint lost its credential across input and release changes: %v, %v", found, err)
	}
	if values := i.Snapshot().Values; values["tenant"] != "other" {
		t.Fatalf("an unchanged endpoint lost its secret input: %v", values)
	}
	moved := declare("3", "https://elsewhere.example.test/tools")
	selectRelease(sameEndpoint, moved)
	requireRows(t, db, map[string]int{"mcp_oauth_sessions": 1})
	if _, _, found, err := authorization.LoadOAuthSession(t.Context(), target(moved)); err != nil || found {
		t.Fatalf("a changed endpoint reused the earlier credential: %v, %v", found, err)
	}
	if values := i.Snapshot().Values; len(values) != 0 {
		t.Fatalf("a changed endpoint kept its secret input: %v", values)
	}
	if err := store.Remove(t.Context(), i.ID()); err != nil {
		t.Fatal(err)
	}
	requireRows(t, db, map[string]int{"mcp_oauth_sessions": 0})
}

func TestInstallationStateRoundTripsRelationally(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	catalog, store := sqlite.NewReleaseStore(db), sqlite.NewInstallationStore(db)
	declaration := plugin.Declaration{
		Name:    "package",
		Servers: []plugin.Server{{Name: testsupport.ServerName("a"), Transport: mcpserver.TransportStdio, Command: "server"}, {Name: testsupport.ServerName("b"), Transport: mcpserver.TransportStdio, Command: "server"}},
		Inputs:  []plugin.Input{{ID: "key", Server: testsupport.ServerName("a"), Target: plugin.Environment, Key: "API_KEY", Secret: true}},
		Skills:  []plugin.Skill{{Name: "review", Description: "Review changes"}},
	}
	release := testsupport.Release(t, "1", declaration)
	declaration.Version = "2"
	candidate := testsupport.Release(t, "2", declaration)
	for _, admitted := range []plugin.Release{release, candidate} {
		if err := catalog.Admit(t.Context(), admitted); err != nil {
			t.Fatal(err)
		}
	}
	i, err := plugin.New(testsupport.InstallationID(t), "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	key := "secret"
	for _, transition := range []error{
		i.Approve(release),
		i.Configure(release, plugin.Configuration{Values: map[string]plugin.ValueChange{"key": plugin.SetValue(key)}, Servers: map[mcpserver.ServerName]plugin.ComponentChange{testsupport.ServerName("b"): plugin.DisableComponent}, Skills: map[string]plugin.ComponentChange{"review": plugin.DisableComponent}}),
		i.Enable(release),
		i.Stage(release, candidate),
	} {
		if transition != nil {
			t.Fatal(transition)
		}
	}
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Get(t.Context(), i.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Installation.Snapshot(), i.Snapshot()) {
		t.Fatalf("relational round trip = %+v, want %+v", restored.Installation.Snapshot(), i.Snapshot())
	}
	if restored.Selected.Digest() != release.Digest() || restored.Staged == nil || restored.Staged.Digest() != candidate.Digest() {
		t.Fatalf("installation read lost its selected or staged release: %+v", restored)
	}
	requireRows(t, db, map[string]int{"plugin_installation_values": 1, "plugin_installation_disabled": 2, "mcp_sources": 2})
	for _, statement := range []string{
		`UPDATE plugin_installations SET admission_state = 'active' WHERE id = ?`,
		`UPDATE plugin_installations SET staged_digest = selected_digest WHERE id = ?`,
		`INSERT INTO plugin_installation_disabled(installation_id, kind, name) VALUES (?, 'theme', 'dark')`,
	} {
		if _, err := db.ExecContext(t.Context(), statement, i.ID().String()); err == nil {
			t.Fatalf("schema admitted an invalid installation state: %s", statement)
		}
	}
	if err := store.Remove(t.Context(), i.ID()); err != nil {
		t.Fatal(err)
	}
	requireRows(t, db, map[string]int{"plugin_installation_values": 0, "plugin_installation_disabled": 0, "mcp_sources": 0})
}

// The installation names releases by digest. Release content has one owner,
// the insert-only catalog, which no installation write can advance.
func TestInstallationNeverStoresAReleaseCopy(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	catalog, store := sqlite.NewReleaseStore(db), sqlite.NewInstallationStore(db)
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{{Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "original"}}})
	if err := catalog.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	id := testsupport.InstallationID(t)
	i, err := plugin.New(id, "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), i); err != nil {
		t.Fatal(err)
	}
	replacement := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{{Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStdio, Command: "replacement"}}})
	if err := catalog.Admit(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	admitted, err := catalog.Get(t.Context(), release.Digest())
	if err != nil || admitted.Declaration().Servers[0].Command != "original" {
		t.Fatalf("catalog advanced an admitted declaration: %+v, %v", admitted, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE plugin_releases SET declaration='{}' WHERE digest=?`, release.Digest().String()); err == nil {
		t.Fatal("admitted declaration was mutable")
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE plugin_installations SET selected_digest=? WHERE id=?`, testsupport.Digest("2").String(), id.String()); err == nil {
		t.Fatal("installation selected an unadmitted release")
	}
}

// Earlier shapes kept installation state in a JSON column, then in an
// approval flag beside an enablement flag. Neither is converted.
func TestOpenRefusesFormerInstallationStateShapes(t *testing.T) {
	for _, former := range []string{"state", "approved"} {
		t.Run(former, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "flame.db")
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), `ALTER TABLE plugin_installations RENAME COLUMN admission_state TO `+former); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if reopened, err := sqlite.Open(t.Context(), path); err == nil {
				_ = reopened.Close()
				t.Fatalf("opened a directory with former installation state column %q", former)
			}
		})
	}
}
