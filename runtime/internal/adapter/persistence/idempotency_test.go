package persistence

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/qimu/portable"
	"github.com/Tangerg/qimu/sqlite"
)

func TestIdempotencyStoreReplayConflictAndExpiry(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newIdempotencyStore(db)
	ctx := context.Background()

	first := operation.ReplayRecord{Key: "request-key", Fingerprint: "first", Payload: []byte(`{"result":1}`)}
	got, claimed, err := store.Claim(ctx, first.Key, first.Fingerprint)
	if err != nil || !claimed {
		t.Fatalf("claim first record: claimed=%v err=%v", claimed, err)
	}
	if len(got.Payload) != 0 {
		t.Fatalf("new claim payload = %q, want empty", got.Payload)
	}
	if _, execContextErr := db.ExecContext(ctx, `UPDATE idempotency_records SET expires_at = 0 WHERE key = ?`, first.Key); execContextErr != nil {
		t.Fatalf("age pending claim: %v", execContextErr)
	}
	got, claimed, err = store.Claim(ctx, first.Key, first.Fingerprint)
	if err != nil || claimed || len(got.Payload) != 0 {
		t.Fatalf("aged pending claim: record=%+v claimed=%v err=%v", got, claimed, err)
	}
	conflicting := operation.ReplayRecord{Key: first.Key, Fingerprint: "second"}
	if _, _, claimErr := store.Claim(ctx, conflicting.Key, conflicting.Fingerprint); !errors.Is(claimErr, portable.ErrInvocationConflict) {
		t.Fatalf("reuse aged pending claim = %v, want ErrKeyConflict", claimErr)
	}
	if completeErr := store.Complete(ctx, first); completeErr != nil {
		t.Fatalf("complete aged pending record: %v", completeErr)
	}
	if err := store.Complete(ctx, first); err != nil {
		t.Fatalf("confirm the same completion: %v", err)
	}
	rewritten := first
	rewritten.Payload = []byte(`{"result":2}`)
	if err := store.Complete(ctx, rewritten); err == nil {
		t.Fatal("accepted a different outcome for the completed request")
	}
	got, claimed, err = store.Claim(ctx, first.Key, first.Fingerprint)
	if err != nil || claimed || string(got.Payload) != string(first.Payload) {
		t.Fatalf("completed claim: record=%+v claimed=%v err=%v", got, claimed, err)
	}
	if _, _, claimErr := store.Claim(ctx, conflicting.Key, conflicting.Fingerprint); !errors.Is(claimErr, portable.ErrInvocationConflict) {
		t.Fatalf("claim conflicting record = %v, want ErrKeyConflict", claimErr)
	}

	if _, execContextErr := db.ExecContext(ctx, `UPDATE idempotency_records SET expires_at = 0 WHERE key = ?`, first.Key); execContextErr != nil {
		t.Fatalf("expire record: %v", execContextErr)
	}
	got, claimed, err = store.Claim(ctx, conflicting.Key, conflicting.Fingerprint)
	if err != nil || !claimed || got.Fingerprint != conflicting.Fingerprint {
		t.Fatalf("replace expired record: record=%+v claimed=%v err=%v", got, claimed, err)
	}
}

func TestIdempotencyStoreKeepsAbandonedClaimAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	store := newIdempotencyStore(db)
	record, claimed, err := store.Claim(t.Context(), "abandoned", "first")
	if err != nil || !claimed {
		t.Fatalf("initial claim = (%+v, %v, %v)", record, claimed, err)
	}
	if _, execContextErr := db.ExecContext(t.Context(),
		`UPDATE idempotency_records SET expires_at = 0 WHERE key = ?`, record.Key,
	); execContextErr != nil {
		t.Fatalf("age pending claim: %v", execContextErr)
	}
	if closeErr := db.Close(); closeErr != nil {
		t.Fatalf("close before reopen: %v", closeErr)
	}

	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = newIdempotencyStore(db)
	got, claimed, err := store.Claim(t.Context(), record.Key, record.Fingerprint)
	if err != nil || claimed || len(got.Payload) != 0 {
		t.Fatalf("claim after reopen = (%+v, %v, %v), want unresolved reservation", got, claimed, err)
	}
	if _, _, err := store.Claim(t.Context(), record.Key, "second"); !errors.Is(err, portable.ErrInvocationConflict) {
		t.Fatalf("reuse after reopen = %v, want ErrKeyConflict", err)
	}
}

func TestIdempotencyNamespaceIdentifiesOneDurableStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	openNamespace := func() (*sql.DB, operation.ReplayNamespace) {
		db, err := openDatabase(t.Context(), path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		namespace, err := idempotencyNamespace(t.Context(), db)
		if err != nil {
			_ = db.Close()
			t.Fatalf("read namespace: %v", err)
		}
		return db, namespace
	}

	db, first := openNamespace()
	if err := db.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}
	db, reopened := openNamespace()
	if reopened != first {
		t.Fatalf("reopened namespace = %q, want %q", reopened.String(), first.String())
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close reopened: %v", err)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("remove recreated-store fixture %q: %v", path+suffix, err)
		}
	}
	db, replaced := openNamespace()
	t.Cleanup(func() { _ = db.Close() })
	if replaced == first {
		t.Fatalf("replacement store reused namespace %q", replaced.String())
	}
}

func TestIdempotencyNamespaceRejectsCorruptIdentity(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(t.Context(),
		`UPDATE runtime_identity SET idempotency_namespace = 'not-an-opaque-identity' WHERE id = 1`,
	); err != nil {
		t.Fatalf("corrupt namespace fixture: %v", err)
	}
	if _, err := idempotencyNamespace(t.Context(), db); err == nil {
		t.Fatal("read corrupt namespace succeeded")
	}
}

func TestAtomicReplaySurvivesFailedWriteAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.db")
	open := func() (*sql.DB, *operation.Replay) {
		t.Helper()
		db, err := openDatabase(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := operation.NewReplay(newIdempotencyStore(db))
		if err != nil {
			t.Fatal(err)
		}
		return db, r
	}
	db, r := open()
	ctx := t.Context()
	interrupt := errors.New("injected commit failure")
	published := 0
	prepare := func(fail bool) func() (operation.PreparedOutcome, error) {
		return func() (operation.PreparedOutcome, error) {
			return operation.PreparedOutcome{Payload: []byte("original"), Commit: func(tx context.Context) error {
				if _, err := sqlite.Connection(tx, db).ExecContext(tx, `INSERT INTO feedback_entries(text,created_at_ns) VALUES ('atomic replay test',1)`); err != nil {
					return err
				}
				if fail {
					return interrupt
				}
				return nil
			}, Publish: func() { published++ }}, nil
		}
	}
	if _, err := r.InvokeAtomic(ctx, "key", "create", prepare(true)); !errors.Is(err, interrupt) {
		t.Fatal(err)
	}
	if published != 0 {
		t.Fatal("rolled-back write was published")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, r = open()
	if body, err := r.InvokeAtomic(ctx, "key", "create", prepare(false)); err != nil || string(body) != "original" {
		t.Fatalf("retry: %q, %v", body, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, r = open()
	defer db.Close()
	if body, err := r.InvokeAtomic(ctx, "key", "create", func() (operation.PreparedOutcome, error) {
		t.Fatal("replayed preparation")
		return operation.PreparedOutcome{}, nil
	}); err != nil || string(body) != "original" {
		t.Fatalf("replay: %q, %v", body, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback_entries WHERE text='atomic replay test'`).Scan(&count); err != nil || count != 1 || published != 1 {
		t.Fatalf("writes=%d publications=%d err=%v", count, published, err)
	}
}
