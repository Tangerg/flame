package managedmcp

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

func TestPreparedCommandRetainsUnknownAdmissionAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	db, err := sqlitedb.OpenSchema(t.Context(), path, []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	input := receiptInput(t, "input")
	prepared, reply, err := PrepareCommand(t.Context(), db, "maintain", "original", input)
	if err != nil || prepared == nil || len(reply) != 0 {
		t.Fatalf("admission=%v, %s, %v", prepared, reply, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlitedb.OpenSchema(t.Context(), path, []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := PrepareCommand(t.Context(), db, "maintain", "original", input); !errors.Is(err, ErrCommandUnconfirmed) {
		t.Fatalf("unknown command restarted work: %v", err)
	}
	if _, _, err := PrepareCommand(t.Context(), db, "changed", "original", input); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("changed method=%v", err)
	}
	if _, _, err := PrepareCommand(t.Context(), db, "maintain", "original", receiptInput(t, "changed")); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("changed input=%v", err)
	}
	if _, err := Command(t.Context(), db, "maintain", "original", "input", input, nil, func(context.Context, string) (string, error) {
		t.Fatal("pending command entered atomic handler again")
		return "", nil
	}); !errors.Is(err, ErrCommandUnconfirmed) {
		t.Fatalf("atomic reentry=%v", err)
	}
}

func TestPreparedCompletionIsAtomicWithItsMutationAndImmutable(t *testing.T) {
	db, err := sqlitedb.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "private.db"), []string{CommandsSchema, `CREATE TABLE counter(value INTEGER NOT NULL)`, `INSERT INTO counter VALUES(0)`})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	input := receiptInput(t, "input")
	prepared, _, err := PrepareCommand(t.Context(), db, "maintain", "original", input)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := pluginsdk.Reply("saved", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER abort_completion BEFORE UPDATE OF reply ON commands BEGIN SELECT RAISE(ABORT,'receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	mutate := func(ctx context.Context) error {
		if _, err := sqlitedb.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=value+1`); err != nil {
			return err
		}
		return prepared.Complete(ctx, reply)
	}
	if err := sqlitedb.RunInTx(t.Context(), db, mutate); err == nil {
		t.Fatal("receipt failure committed mutation")
	}
	var count int
	var saved sql.NullString
	if err := db.QueryRowContext(t.Context(), `SELECT value FROM counter`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("mutation survived failed receipt: %d, %v", count, err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT reply FROM commands WHERE id='original'`).Scan(&saved); err != nil || saved.Valid {
		t.Fatalf("failed receipt became known: %v, %v", saved, err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TRIGGER abort_completion`); err != nil {
		t.Fatal(err)
	}
	if err := sqlitedb.RunInTx(t.Context(), db, mutate); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Complete(t.Context(), reply); err != nil {
		t.Fatal(err)
	}
	other, err := pluginsdk.Reply("different", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Complete(t.Context(), changed); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("completed outcome changed: %v", err)
	}
	duplicate, original, err := PrepareCommand(t.Context(), db, "maintain", "original", input)
	if err != nil || duplicate != nil || string(original) != string(reply) {
		t.Fatalf("original result=%v, %s, %v", duplicate, original, err)
	}
}
