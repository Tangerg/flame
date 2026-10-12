package sqlite_test

import (
	"errors"
	"path/filepath"
	"testing"

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
	contract := testsupport.JSONInput(t, `{"name":"review.update","kind":"command","target":"workspace","input":{"type":"object"},"output":{"type":"object"}}`)
	command, err := plugin.NewCommand("command", "review.update", testsupport.JSONInput(t, `{"revision":2}`), contract, provider, testsupport.ServerName("backend"))
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
	if err != nil || !found || restored.Snapshot() != command.Snapshot() {
		t.Fatalf("original binding=%v, %v, %v", restored, found, err)
	}
	held, err := store.PendingCommandDependencies(t.Context())
	if err != nil || len(held) != 1 || held[0] != provider {
		t.Fatalf("durable dependency=%v, %v", held, err)
	}
	changed := command.Snapshot()
	changed.Provider.Digest = testsupport.Digest("3")
	other, err := plugin.RestoreCommand(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), other); !errors.Is(err, plugin.ErrCommandConflict) {
		t.Fatalf("retargeting=%v", err)
	}
	changed = command.Snapshot()
	changed.Contract = testsupport.JSONInput(t, `{"name":"review.update","kind":"command","target":"another"}`)
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
INSERT INTO plugin_commands SELECT 'command-'||n,'review.update',?,?,?,?,'backend','dispatched' FROM slots`, 4096, testsupport.JSONInput(t, `{"revision":2}`).String(), testsupport.JSONInput(t, `{"name":"review.update"}`).String(), provider.InstallationID.String(), provider.Digest.String())
	if err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewPluginCommandStore(db)
	command, err := plugin.NewCommand("new", "review.update", testsupport.JSONInput(t, `{"revision":2}`), testsupport.JSONInput(t, `{"name":"review.update"}`), provider, testsupport.ServerName("backend"))
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
