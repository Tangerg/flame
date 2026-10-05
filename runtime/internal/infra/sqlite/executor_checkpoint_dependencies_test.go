package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func testInstallationID(t *testing.T, text string) resourceid.InstallationID {
	t.Helper()
	id, err := resourceid.ParseInstallation(text)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertRunState(t *testing.T, db *sql.DB, runID, sessionID, state string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO runs(run_id, session_id, state, created_at, updated_at) VALUES (?, ?, ?, 0, 0)`,
		runID, sessionID, state,
	); err != nil {
		t.Fatal(err)
	}
}

// The payload is deliberately not JSON: answering the dependency question must
// never require interpreting any continuation, valid or not.
func TestCheckpointDependencyQueryReadsOnlyTheProjection(t *testing.T) {
	checkpointInstallation := testInstallationID(t, "940ac827-b431-455b-af4b-e3a170bcfda0")
	otherInstallation := testInstallationID(t, "1d7f3c2e-6a1b-4f0e-9c3d-2b8a7e5f4c10")
	db, store := newExecutorCheckpointStorage(t)
	storage := sqlite.NewExecutorCheckpointStore(db)
	dependency := plugin.Dependency{InstallationID: checkpointInstallation, Digest: testsupport.Digest("1")}
	waiting := storedExecutorCheckpoint("member_waiting", "ses_waiting", "\x00not a continuation")
	waiting.Installations = []plugin.Dependency{dependency}
	finished := storedExecutorCheckpoint("member_finished", "ses_finished", "{")
	finished.Installations = []plugin.Dependency{{InstallationID: otherInstallation, Digest: testsupport.Digest("2")}}
	insertRunState(t, db, "run_waiting", "ses_waiting", "waiting")
	insertRunState(t, db, "run_finished", "ses_finished", "terminal")
	if err := store.SaveCheckpoint(t.Context(), waiting); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCheckpoint(t.Context(), finished); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		installation resourceid.InstallationID
		want         bool
	}{
		{checkpointInstallation, true},
		{otherInstallation, false},
	} {
		got, err := pendingDependsOn(t, store, test.installation)
		if err != nil || got != test.want {
			t.Fatalf("depends on %s = %v, %v; want %v", test.installation, got, err, test.want)
		}
	}
	loaded, err := store.LoadCheckpoint(t.Context(), "member_waiting")
	if err != nil || !slices.Equal(loaded.Installations, waiting.Installations) {
		t.Fatalf("loaded dependencies = %+v, %v", loaded.Installations, err)
	}

	advanced := waiting.Clone()
	advanced.Installations = nil
	if err := store.SaveCheckpoint(t.Context(), advanced); err != nil {
		t.Fatal(err)
	}
	if got, err := pendingDependsOn(t, storage, checkpointInstallation); err != nil || got {
		t.Fatalf("advanced checkpoint retained a released dependency: %v, %v", got, err)
	}
	waiting.Installations = []plugin.Dependency{dependency}
	if err := store.SaveCheckpoint(t.Context(), waiting); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteCheckpoints(t.Context(), "ses_waiting", []string{"member_waiting"}); err != nil {
		t.Fatal(err)
	}
	if got, err := pendingDependsOn(t, storage, checkpointInstallation); err != nil || got {
		t.Fatalf("deleted checkpoint retained its dependency: %v, %v", got, err)
	}
}

func TestCheckpointStoreRejectsNonCanonicalDependencies(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	dependency := plugin.Dependency{InstallationID: testsupport.InstallationID(t), Digest: testsupport.Digest("1")}
	checkpoint := storedExecutorCheckpoint("member_duplicate", "ses_duplicate", "payload")
	checkpoint.Installations = []plugin.Dependency{dependency, dependency}
	if err := store.SaveCheckpoint(t.Context(), checkpoint); err == nil {
		t.Fatal("duplicate dependency projection was stored")
	}
}

func TestPendingCheckpointWithoutDependencyProjectionRefusesTheDirectory(t *testing.T) {
	for _, test := range []struct {
		state   string
		refused bool
	}{
		{state: "waiting", refused: true},
		{state: "terminal"},
	} {
		t.Run(test.state, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "flame.db")
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			store := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
			if err := store.SaveCheckpoint(t.Context(), storedExecutorCheckpoint("member_old", "ses_old", "payload")); err != nil {
				t.Fatal(err)
			}
			insertRunState(t, db, "run_old", "ses_old", test.state)
			if _, err := db.ExecContext(t.Context(), `DROP TABLE executor_checkpoint_installations`); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := sqlite.Open(t.Context(), path)
			if reopened != nil {
				_ = reopened.Close()
			}
			if test.refused != (err != nil) {
				t.Fatalf("reopen with %s checkpoint = %v, want refused=%v", test.state, err, test.refused)
			}
		})
	}
}

type pendingDependencies interface {
	PendingCheckpointDependencies(context.Context) ([]plugin.Dependency, error)
}

func pendingDependsOn(t *testing.T, store pendingDependencies, installation resourceid.InstallationID) (bool, error) {
	t.Helper()
	dependencies, err := store.PendingCheckpointDependencies(t.Context())
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(dependencies, func(dependency plugin.Dependency) bool {
		return dependency.InstallationID == installation
	}), nil
}
