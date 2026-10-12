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
		{"former installation trust state", `ALTER TABLE plugin_installations RENAME COLUMN enabled TO admission_state`},
		{"retired object", `CREATE INDEX idx_feedback_entries_created ON feedback_entries(created_at_ns DESC)`},
		{"missing table", `DROP TABLE automation_starts`},
		{"former host without command routing", `DROP TABLE plugin_commands`},
		{"former host command digest", `ALTER TABLE plugin_commands RENAME COLUMN input_json TO input_digest`},
		{"former command without bound contract", `ALTER TABLE plugin_commands DROP COLUMN contract_json`},
		{"former host without provider selections", `DROP TABLE plugin_provider_selections`},
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
