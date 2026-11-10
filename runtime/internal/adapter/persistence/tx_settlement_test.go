package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/frame/sqlite"
)

// Fault injection is at the adapter's owner boundary. Frame alone owns the
// physical transaction, driver policy, cursor joining and settlement evidence.
type settlementBoundary struct {
	databaseBoundary
	readFailure  error
	writeFailure error
}

func (b *settlementBoundary) read(ctx context.Context, read func(sqlite.Reader) error) error {
	return errors.Join(b.databaseBoundary.read(ctx, read), b.readFailure)
}
func (b *settlementBoundary) write(ctx context.Context, write func(context.Context, sqlite.Executor) error) error {
	return errors.Join(b.databaseBoundary.write(ctx, write), b.writeFailure)
}

func settlementFixture(t *testing.T) (*database, *settlementBoundary) {
	t.Helper()
	raw, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	boundary := &settlementBoundary{databaseBoundary: independentDatabase{db: raw}}
	return &database{boundary: boundary}, boundary
}

func TestMissingInstallationReadRequiresAcceptedSettlement(t *testing.T) {
	failure := errors.New("installation read settlement failed")
	db, boundary := settlementFixture(t)
	boundary.readFailure = failure
	snapshot, found, err := newInstallationStore(newReleaseStore(db)).Get(t.Context(), testsupport.InstallationID(t))
	if found || !errors.Is(err, failure) {
		t.Fatalf("installation = (%+v, %t, %v), want rejected read settlement", snapshot, found, err)
	}
}

func TestInstallationLookupSeparatesAbsentRecordFromMissingRelease(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	catalog := newReleaseStore(databaseFor(db))
	store := newInstallationStore(catalog)
	id := testsupport.InstallationID(t)
	if _, found, err := store.Get(t.Context(), id); err != nil || found {
		t.Fatalf("absent installation = (%t, %v), want admitted absence", found, err)
	}
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package"})
	if err := catalog.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(id, "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.projectRecord(t.Context(), installation.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Get(t.Context(), id); err != nil || !found {
		t.Fatalf("admitted installation = (%t, %v)", found, err)
	}
	db = corruptFixture(t, db, func(connection *sql.Conn) error {
		if _, err := connection.ExecContext(t.Context(), `PRAGMA foreign_keys=OFF`); err != nil {
			return err
		}
		_, err := connection.ExecContext(t.Context(), `DELETE FROM plugin_releases WHERE digest=?`, release.Digest().String())
		return err
	})
	store = newInstallationStore(newReleaseStore(databaseFor(db)))
	if snapshot, found, err := store.Get(t.Context(), id); found || !errors.Is(err, plugin.ErrNotFound) || !strings.Contains(err.Error(), "selected release") {
		t.Fatalf("malformed installation = (%v, %t, %v), want rejected restoration with release cause", snapshot.Installation, found, err)
	}
}

func TestWorkspaceIntentRejectionRetainsSettlementFailure(t *testing.T) {
	retirement := errors.New("workspace intent settlement failed")
	db, boundary := settlementFixture(t)
	boundary.writeFailure = retirement
	created, err := newWorkspaceMutationStore(db).recordMutation(t.Context(), workspaceMutationRecord{SessionID: "ses_missing", CWD: t.TempDir(), ToRunID: "run_missing"})
	if created || !errors.Is(err, retirement) || !strings.Contains(err.Error(), "no matching Run boundary") {
		t.Fatalf("intent = (%t, %v), want rejection and settlement failure", created, err)
	}
}

func TestAdoptedWorkspaceIntentRequiresTransactionSettlement(t *testing.T) {
	db, boundary := settlementFixture(t)
	raw := boundary.databaseBoundary.(independentDatabase).db
	seedWorkspaceMutationSession(t, raw, "ses_intent", "/repo")
	seedWorkspaceMutationRun(t, raw, "ses_intent", "run_intent")
	store := newWorkspaceMutationStore(db)
	intent := workspaceMutationRecord{SessionID: "ses_intent", CWD: "/repo", ToRunID: "run_intent"}
	if created, err := store.recordMutation(t.Context(), intent); !created || err != nil {
		t.Fatalf("initial intent = (%t, %v)", created, err)
	}
	failure := errors.New("intent read transaction settlement failed")
	boundary.writeFailure = failure
	if created, err := store.recordMutation(t.Context(), intent); created || !errors.Is(err, failure) {
		t.Fatalf("adopted intent = (%t, %v), want retained settlement failure", created, err)
	}
}
