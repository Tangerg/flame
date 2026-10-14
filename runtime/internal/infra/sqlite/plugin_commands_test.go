package sqlite_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestPluginCommandBindingSurvivesRestartAndCannotRetarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewPluginCommandStore(db)
	provider := plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1")}
	contract := testsupport.CommandContract(t, "review.update", "workspace")
	leaf := plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("leaf")}
	dependencies := plugin.CompactDependencies([]plugin.Dependency{provider, leaf})
	command, err := plugin.NewCommand(plugin.CommandBinding{ID: "command", Invocation: capability.Invocation{ID: "command", Origin: capability.Human, Target: capability.Target{Scope: "workspace", Reference: "workspace:/review"}}, Method: "review.update", Input: testsupport.JSONInput(t, `{"revision":2}`), Contract: contract, Provider: provider, Activation: plugin.Activation{Identity: testsupport.Digest("activation"), Dependencies: dependencies}, Server: testsupport.ServerName("backend"), Contribution: "original-provider", Export: "operations"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	command.Dispatch()
	if err := store.Save(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = sqlite.NewPluginCommandStore(db)
	restored, found, err := store.Get(t.Context(), "command")
	if err != nil || !found || !reflect.DeepEqual(restored.Snapshot(), command.Snapshot()) {
		t.Fatalf("original binding=%v, %v, %v", restored, found, err)
	}
	held, err := store.PendingCommandDependencies(t.Context())
	if err != nil || !slices.Equal(held, dependencies) {
		t.Fatalf("durable dependency=%v, %v", held, err)
	}
	copy := restored.Snapshot()
	copy.Activation.Dependencies[0] = plugin.Dependency{}
	if !slices.Equal(restored.Snapshot().Activation.Dependencies, dependencies) {
		t.Fatal("caller changed retained dependencies")
	}
	changedIdentity := command.Snapshot()
	changedIdentity.Activation.Identity = testsupport.Digest("same releases with another composition")
	otherActivation, err := plugin.RestoreCommand(changedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), otherActivation); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("activation identity changed: %v", err)
	}
	changedDependencies := command.Snapshot()
	changedDependencies.Activation.Dependencies = []plugin.Dependency{provider}
	otherGraph, err := plugin.RestoreCommand(changedDependencies)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), otherGraph); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("transitive dependencies changed: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE plugin_commands SET activation_json='{}' WHERE id='command'`); err == nil {
		t.Fatal("SQL rewrote the original activation dependencies")
	}
	changed := command.Snapshot()
	changed.Provider.Digest = testsupport.Digest("3")
	changed.Activation.Dependencies = []plugin.Dependency{changed.Provider}
	other, err := plugin.RestoreCommand(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), other); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("retargeting=%v", err)
	}
	changed = command.Snapshot()
	changed.Contract = testsupport.CommandContract(t, "review.update", "another")
	changed.Invocation.Target.Scope = "another"
	other, err = plugin.RestoreCommand(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), other); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("contract replacement=%v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE plugin_commands SET contract_json='{}' WHERE id='command'`); err == nil {
		t.Fatal("SQL changed the original contract")
	}
	if err := restored.Observe(); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), restored); err != nil {
		t.Fatal(err)
	}
	held, err = store.PendingCommandDependencies(t.Context())
	if err != nil || len(held) != 0 {
		t.Fatalf("observed command still holds a dependency=%v, %v", held, err)
	}
	retained, found, err := store.Get(t.Context(), "command")
	if err != nil || !found || retained.Snapshot().Provider != provider {
		t.Fatalf("observation erased the original route=%v, %v", retained, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE plugin_commands SET phase='prepared' WHERE id='command'`); err == nil {
		t.Fatal("command phase regressed")
	}
}

func TestPluginCommandHistoryNeverEvictsEarlierIdentityOrCapsLifetimeUsage(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	provider := plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1")}
	_, err = db.ExecContext(t.Context(), `WITH RECURSIVE slots(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM slots WHERE n<?)
INSERT INTO plugin_commands(id,method,business_input_json,contract_json,installation_id,release_digest,server_name,contribution_id,export_name,invocation_json,activation_json,phase) SELECT 'command-'||n,'review.update',?,?,?,?,'backend','original-provider','operations',json_object('id','command-'||n,'origin','human','target',json_object('scope','installation','reference',?)),json_object('identity',?,'dependencies',json_array(json_object('installation',?,'digest',?))),'dispatched' FROM slots`, 4096, testsupport.JSONInput(t, `{"revision":2}`).String(), testsupport.CommandContract(t, "review.update", "installation").String(), provider.InstallationID.String(), provider.Digest.String(), provider.InstallationID.String(), testsupport.Digest("activation").String(), provider.InstallationID.String(), provider.Digest.String())
	if err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewPluginCommandStore(db)
	command, err := plugin.NewCommand(plugin.CommandBinding{ID: "new", Invocation: capability.Invocation{ID: "new", Origin: capability.Human, Target: capability.Target{Scope: "installation", Reference: provider.InstallationID.String()}}, Method: "review.update", Input: testsupport.JSONInput(t, `{"revision":2}`), Contract: testsupport.CommandContract(t, "review.update", "installation"), Provider: provider, Activation: plugin.Activation{Identity: testsupport.Digest("activation"), Dependencies: []plugin.Dependency{provider}}, Server: testsupport.ServerName("backend"), Contribution: "original-provider", Export: "operations"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), command); err != nil {
		t.Fatalf("historical commands blocked a fresh command: %v", err)
	}
	previous, found, err := store.Get(t.Context(), "command-1")
	if err != nil || !found {
		t.Fatalf("old identity was evicted: %v", err)
	}
	if err := previous.Observe(); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), previous); err != nil {
		t.Fatalf("history blocked existing reconciliation: %v", err)
	}
}
