package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestOpenRejectsObsoleteApprovalSchemaWithoutChangingData(t *testing.T) {
	for _, schema := range []struct{ name, create, insert, column, value string }{
		{"name identity", `CREATE TABLE approval_rules (id TEXT PRIMARY KEY, scope TEXT, scope_key TEXT, tool TEXT, subject TEXT, decision TEXT)`, `INSERT INTO approval_rules VALUES ('old','global','','shell','','allow')`, "tool", "shell"},
		{"implicit subject", `CREATE TABLE approval_rules (id TEXT PRIMARY KEY, scope TEXT, scope_key TEXT, tool_ref TEXT, source_fingerprint TEXT, mcp_server TEXT, subject TEXT, decision TEXT)`, `INSERT INTO approval_rules VALUES ('old','global','','builtIn:shell','',NULL,'echo *','allow')`, "subject", "echo *"},
	} {
		t.Run(schema.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "obsolete.db")
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range []string{
				schema.create, schema.insert,
			} {
				if _, err := raw.ExecContext(t.Context(), statement); err != nil {
					t.Fatal(err)
				}
			}
			if err := raw.Close(); err != nil {
				t.Fatal(err)
			}
			opened, err := sqlite.Open(t.Context(), path)
			if err == nil {
				opened.Close()
				t.Fatal("obsolete authorization schema was accepted")
			}
			if !strings.Contains(err.Error(), "incompatible approval schema") {
				t.Fatalf("unexpected error: %v", err)
			}
			raw, err = sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			var name string
			if err := raw.QueryRowContext(t.Context(), "SELECT "+schema.column+" FROM approval_rules WHERE id='old'").Scan(&name); err != nil || name != schema.value {
				t.Fatalf("rejected open changed stored rule: %q, %v", name, err)
			}

		})
	}
}
