package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// TestOpenRetiresObsoleteSchemaOnAnExistingDatabase covers the half a fresh
// install cannot: a database that already carries the retired objects. Open is
// the only migration point, so a removal that only edits the CREATE statements
// would leave every existing installation paying for them forever.
func TestOpenRetiresObsoleteSchemaOnAnExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	first, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Reinstate exactly what older installations have.
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_feedback_entries_created ON feedback_entries(created_at DESC)`,
		`ALTER TABLE pending_workspace_mutations ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := first.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("reinstate %q: %v", stmt, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if count := scalar(t, db, `SELECT count(*) FROM pragma_index_list('feedback_entries') WHERE name = ?`,
		"idx_feedback_entries_created"); count != 0 {
		t.Fatal("reopen kept the feedback index that serves no reader")
	}
	if count := scalar(t, db, `SELECT count(*) FROM pragma_table_info('pending_workspace_mutations') WHERE name = ?`,
		"created_at"); count != 0 {
		t.Fatal("reopen kept the storage-minted clock on the pending rollback log")
	}
	if _, err := sqlite.NewWorkspaceMutationStore(db).ListPending(t.Context()); err != nil {
		t.Fatalf("ListPending after retirement: %v", err)
	}
}

func scalar(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var value int
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}
