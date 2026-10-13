package managedmcp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestReadCommandUsesOnlyOriginalCommittedReceipt(t *testing.T) {
	db, err := sqlite.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "backend.db"), []string{CommandsSchema, `CREATE TABLE effects(value TEXT)`})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	input := map[string]string{"second": "2", "first": "1"}
	if _, found, err := ReadCommand[map[string]string, string](t.Context(), db, "create", "original", input); err != nil || found {
		t.Fatalf("absent receipt: %t,%v", found, err)
	}
	value, err := Command(t.Context(), db, "create", "original", input, func(ctx context.Context, _ map[string]string) (string, error) {
		_, err := sqlite.Connection(ctx, db).ExecContext(ctx, `INSERT INTO effects VALUES('created')`)
		return "historical result", err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM effects`); err != nil {
		t.Fatal(err)
	}
	reply, found, err := ReadCommand[map[string]string, string](t.Context(), db, "create", "original", map[string]string{"first": "1", "second": "2"})
	if err != nil || !found || reply != value {
		t.Fatalf("historical receipt: %q,%t,%v", reply, found, err)
	}
	for _, wrong := range []struct {
		method, id string
		input      map[string]string
	}{{"different", "original", input}, {"create", "original", map[string]string{"first": "other"}}} {
		if _, found, err := ReadCommand[map[string]string, string](t.Context(), db, wrong.method, wrong.id, wrong.input); found || !errors.Is(err, idempotency.ErrKeyConflict) {
			t.Fatalf("conflict became a result: %t,%v", found, err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE commands SET result='{"unexpected":"bad"}' WHERE id='original'`); err != nil {
		t.Fatal(err)
	}
	if _, found, err := ReadCommand[map[string]string, struct{ Expected string }](t.Context(), db, "create", "original", input); found || err == nil {
		t.Fatalf("malformed receipt: %t,%v", found, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, found, err := ReadCommand[map[string]string, string](ctx, db, "create", "original", input); found || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lookup: %t,%v", found, err)
	}
	var effects, receipts int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM effects`).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if effects != 0 || receipts != 1 {
		t.Fatalf("queries changed backend state: effects=%d receipts=%d", effects, receipts)
	}
}
