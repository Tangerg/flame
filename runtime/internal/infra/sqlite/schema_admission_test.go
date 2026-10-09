package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

// A data directory is empty or exactly current. A former column, a retired
// object or a missing table is refused before any statement runs, and the
// directory's rows are left exactly as they were: there is no migration path.
func TestOpenAdmitsOnlyTheExactCurrentSchema(t *testing.T) {
	for _, change := range []struct{ name, statement string }{
		{"former column", `ALTER TABLE model_invocations DROP COLUMN usage`},
		{"retired object", `CREATE INDEX idx_feedback_entries_created ON feedback_entries(created_at_ns DESC)`},
		{"missing table", `DROP TABLE agent_memory_state`},
		{"former Tool result time unit", `ALTER TABLE tool_result_blobs RENAME COLUMN created_at_ns TO created_at`},
		{"former feedback time unit", `ALTER TABLE feedback_entries RENAME COLUMN created_at_ns TO created_at`},
	} {
		t.Run(change.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "flame.db")
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range []string{change.statement, `INSERT INTO trusted_projects VALUES ('/retained')`} {
				if _, err := db.ExecContext(t.Context(), statement); err != nil {
					t.Fatalf("%s: %v", statement, err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			reopened, err := sqlite.Open(t.Context(), path)
			if err == nil {
				_ = reopened.Close()
				t.Fatal("a directory whose schema differs from the current one was opened")
			}
			if !strings.Contains(err.Error(), "open a fresh data directory") {
				t.Fatalf("refusal = %v", err)
			}
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			var root string
			if err := raw.QueryRowContext(t.Context(), `SELECT project_root FROM trusted_projects`).Scan(&root); err != nil || root != "/retained" {
				t.Fatalf("refused open changed the directory: %q, %v", root, err)
			}
		})
	}
}

func TestOpenReopensTheCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	for range 2 {
		db, err := sqlite.Open(t.Context(), path)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
