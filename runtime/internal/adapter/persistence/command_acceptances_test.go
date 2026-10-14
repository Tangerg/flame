package persistence_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func sampleAcceptance() runs.CommandAcceptance {
	return runs.CommandAcceptance{
		Key:         "start-once",
		Fingerprint: strings.Repeat("a", 64),
		Operation:   runs.CommandStartOpening,
		SessionID:   "session_accept", RunID: "run_accept",
		SegmentID: "segment_accept", UserItemID: "item_accept",
	}
}

// TestCommandAcceptanceSurvivesReopenAndRetiresWithItsRun is the durable proof of
// the ownership fix: once the opening transaction records an acceptance, a process
// restart still resolves the key to its Run, and deleting the Run retires the
// material to NULL without erasing the recorded identity — which importing the same
// RunID cannot refill.
func TestCommandAcceptanceSurvivesReopenAndRetiresWithItsRun(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "flame.db")
	acceptance := sampleAcceptance()

	db, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	admit := run.Draft{
		RunID: acceptance.RunID, SessionID: acceptance.SessionID, SegmentID: acceptance.SegmentID,
		ModelSelection: testsupport.MustModelSelection("test", "test-model"), CreatedAt: time.Unix(100, 0).UTC(),
	}
	if err := sqlite.NewRunStore(db).Admit(ctx, admit); err != nil {
		t.Fatalf("admit run: %v", err)
	}
	if err := persistence.NewCommandAcceptanceStore(db).Save(ctx, acceptance); err != nil {
		t.Fatalf("save acceptance: %v", err)
	}
	// A second write under the same key is the global-key refusal that keeps a
	// retried opening from admitting a second Run.
	if err := persistence.NewCommandAcceptanceStore(db).Save(ctx, acceptance); err == nil {
		t.Fatal("re-saving the same key must violate the global command key")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	store := persistence.NewCommandAcceptanceStore(reopened)

	record, err := store.Lookup(ctx, acceptance.Key)
	if err != nil {
		t.Fatalf("lookup after reopen: %v", err)
	}
	if !record.Found || record.CurrentRun == nil || !record.Acceptance.Equal(acceptance) {
		t.Fatalf("reopened record = %+v, want the original opening with live material", record)
	}

	if _, err := reopened.ExecContext(ctx, `DELETE FROM runs WHERE run_id = ?`, acceptance.RunID); err != nil {
		t.Fatalf("delete run material: %v", err)
	}
	retired, err := store.Lookup(ctx, acceptance.Key)
	if err != nil {
		t.Fatalf("lookup after retirement: %v", err)
	}
	if !retired.Found || retired.CurrentRun != nil {
		t.Fatalf("retired record = %+v, want the identity without live material", retired)
	}
	if !retired.Acceptance.Equal(acceptance) {
		t.Fatalf("retirement changed the recorded identity: %+v", retired.Acceptance)
	}

	// Importing the same RunID must not refill the retired material reference.
	if err := sqlite.NewRunStore(reopened).Admit(ctx, admit); err != nil {
		t.Fatalf("re-admit run: %v", err)
	}
	refilled, err := store.Lookup(ctx, acceptance.Key)
	if err != nil {
		t.Fatalf("lookup after re-admit: %v", err)
	}
	if refilled.CurrentRun != nil {
		t.Fatal("re-admitting the same RunID refilled a retired acceptance")
	}
}

// TestCommandAcceptanceLookupReportsForeignOperation proves a key held by the
// generic response variant is reported as another operation's, so a run-opening
// retry refuses rather than misreading the row as its own acceptance.
func TestCommandAcceptanceLookupReportsForeignOperation(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx,
		`INSERT INTO idempotency_records(key, kind, fingerprint, payload, expires_at)
		 VALUES ('cancel-once', 'response', 'fp', x'00', 9999999999)`,
	); err != nil {
		t.Fatalf("seed generic response: %v", err)
	}
	record, err := persistence.NewCommandAcceptanceStore(db).Lookup(ctx, "cancel-once")
	if err != nil {
		t.Fatalf("lookup foreign key: %v", err)
	}
	if record.Found || !record.OtherOperation {
		t.Fatalf("foreign-operation record = %+v, want OtherOperation", record)
	}
}
