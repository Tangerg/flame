package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func testInstallationID(t *testing.T, text string) identity.InstallationID {
	t.Helper()
	id, err := identity.ParseInstallation(text)
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
	storage := newExecutorCheckpointStore(db)
	dependency := plugin.Dependency{InstallationID: checkpointInstallation, Digest: testsupport.Digest("1")}
	waiting := storedExecutorCheckpoint("member_waiting", "ses_waiting", "\x00not a continuation")
	waiting.Resources = []plugin.Dependency{dependency}
	finished := storedExecutorCheckpoint("member_finished", "ses_finished", "{")
	finished.Resources = []plugin.Dependency{{InstallationID: otherInstallation, Digest: testsupport.Digest("2")}}
	insertRunState(t, db, "run_waiting", "ses_waiting", "waiting")
	insertRunState(t, db, "run_finished", "ses_finished", "terminal")
	if err := store.SaveCheckpoint(t.Context(), waiting); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCheckpoint(t.Context(), finished); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		installation identity.InstallationID
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
	if err != nil || !slices.Equal(loaded.Resources, waiting.Resources) {
		t.Fatalf("loaded dependencies = %+v, %v", loaded.Resources, err)
	}

	advanced := waiting.Clone()
	advanced.Resources = nil
	if err := store.SaveCheckpoint(t.Context(), advanced); err != nil {
		t.Fatal(err)
	}
	if got, err := pendingDependsOn(t, storage, checkpointInstallation); err != nil || got {
		t.Fatalf("advanced checkpoint retained a released dependency: %v, %v", got, err)
	}
	waiting.Resources = []plugin.Dependency{dependency}
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
	checkpoint.Resources = []plugin.Dependency{dependency, dependency}
	if err := store.SaveCheckpoint(t.Context(), checkpoint); err == nil {
		t.Fatal("duplicate dependency projection was stored")
	}
}

// A directory without the dependency projection predates it; whatever its
// checkpoints hold, it is refused rather than completed in place.
func TestCheckpointDirectoryWithoutDependencyProjectionIsRefused(t *testing.T) {
	for _, test := range []struct{ state string }{{state: "waiting"}, {state: "terminal"}} {
		t.Run(test.state, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "flame.db")
			db, err := openDatabase(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			store := newExecutorCheckpointStore(db)
			if err := store.SaveCheckpoint(t.Context(), storedExecutorCheckpoint("member_old", "ses_old", "payload")); err != nil {
				t.Fatal(err)
			}
			insertRunState(t, db, "run_old", "ses_old", test.state)
			if _, err := db.ExecContext(t.Context(), `DROP TABLE executor_checkpoint_resources`); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := openDatabase(t.Context(), path)
			if reopened != nil {
				_ = reopened.Close()
			}
			if err == nil {
				t.Fatalf("reopen with a %s checkpoint and no dependency projection was accepted", test.state)
			}
		})
	}
}

type pendingDependencies interface {
	PendingCheckpointDependencies(context.Context) ([]plugin.Dependency, error)
}

func pendingDependsOn(t *testing.T, store pendingDependencies, installation identity.InstallationID) (bool, error) {
	t.Helper()
	dependencies, err := store.PendingCheckpointDependencies(t.Context())
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(dependencies, func(dependency plugin.Dependency) bool {
		return dependency.InstallationID == installation
	}), nil
}
