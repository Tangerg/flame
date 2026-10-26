package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
)

func TestPreparedCommandRetainsUnknownAdmissionAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	db, err := sqlite.OpenSchema(t.Context(), path, []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	input := receiptInput(t, "input")
	prepared, reply, err := AdmitCommand(t.Context(), db, "maintain", "original", input, nil)
	if err != nil || prepared == nil || len(reply) != 0 {
		t.Fatalf("admission=%v, %s, %v", prepared, reply, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.OpenSchema(t.Context(), path, []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if pending, reply, err := AdmitCommand(t.Context(), db, "maintain", "original", input, nil); err != nil || pending == nil || pending.NewlyAdmitted() || len(reply) != 0 {
		t.Fatalf("pending command lost its reconciliation-only admission: %v, %s, %v", pending, reply, err)
	}
	if _, _, err := AdmitCommand(t.Context(), db, "changed", "original", input, nil); !errors.Is(err, extension.ErrInvocationConflict) {
		t.Fatalf("changed method=%v", err)
	}
	if _, _, err := AdmitCommand(t.Context(), db, "maintain", "original", receiptInput(t, "changed"), nil); !errors.Is(err, extension.ErrInvocationConflict) {
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
	db, err := sqlite.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "private.db"), []string{CommandsSchema, `CREATE TABLE counter(value INTEGER NOT NULL)`, `INSERT INTO counter VALUES(0)`})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	input := receiptInput(t, "input")
	prepared, _, err := AdmitCommand(t.Context(), db, "maintain", "original", input, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := pluginsdk.EncodeReply("saved", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reply := outcome.JSON()
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER abort_completion BEFORE UPDATE OF reply ON commands BEGIN SELECT RAISE(ABORT,'receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	mutate := func(ctx context.Context) error {
		if _, err := sqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=value+1`); err != nil {
			return err
		}
		return prepared.Complete(ctx, reply)
	}
	if err := sqlite.RunInTx(t.Context(), db, mutate); err == nil {
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
	if err := sqlite.RunInTx(t.Context(), db, mutate); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Complete(t.Context(), reply); err != nil {
		t.Fatal(err)
	}
	other, err := pluginsdk.EncodeReply("different", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed := other.JSON()
	if err := prepared.Complete(t.Context(), changed); !errors.Is(err, extension.ErrInvocationConflict) {
		t.Fatalf("completed outcome changed: %v", err)
	}
	duplicate, original, err := AdmitCommand(t.Context(), db, "maintain", "original", input, nil)
	if err != nil || duplicate != nil || string(original) != string(reply) {
		t.Fatalf("original result=%v, %s, %v", duplicate, original, err)
	}
}
