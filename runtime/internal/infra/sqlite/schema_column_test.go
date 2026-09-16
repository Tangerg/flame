package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func modelInvocationSchema(t *testing.T, db *sql.DB) string {
	t.Helper()
	var text string
	if err := db.QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'model_invocations'`,
	).Scan(&text); err != nil {
		t.Fatalf("read model_invocations schema: %v", err)
	}
	return text
}

// columnDefinition extracts one column's complete definition, constraint and
// all, from a stored CREATE TABLE.
func columnDefinition(t *testing.T, schema, column string) string {
	t.Helper()
	for _, line := range strings.Split(schema, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
		if strings.HasPrefix(trimmed, column+" ") {
			return trimmed
		}
	}
	t.Fatalf("schema has no %q column:\n%s", column, schema)
	return ""
}

// TestMigratedColumnsCarryTheDefinitionAFreshDatabaseGets pins that a column
// added to an existing database and the same column in a new one are one
// definition. They are written at two sites — the CREATE TABLE and the ALTER
// that repairs an older shape — and a divergence between them would leave two
// populations of durable rows under different CHECK constraints, which no query
// reports and no decode notices.
func TestMigratedColumnsCarryTheDefinitionAFreshDatabaseGets(t *testing.T) {
	ctx := t.Context()
	columns := []string{"first_output_latency_millis", "usage"}

	fresh, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	freshSchema := modelInvocationSchema(t, fresh)

	path := filepath.Join(t.TempDir(), "migrated.db")
	old, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		if _, err := old.ExecContext(ctx,
			"ALTER TABLE model_invocations DROP COLUMN "+column,
		); err != nil {
			t.Fatalf("simulate an older shape without %q: %v", column, err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen and repair: %v", err)
	}
	t.Cleanup(func() { _ = migrated.Close() })
	migratedSchema := modelInvocationSchema(t, migrated)

	// A repaired database writes its added columns inline rather than on their
	// own line, so the fresh definition is the text to look for, not to align.
	for _, column := range columns {
		want := columnDefinition(t, freshSchema, column)
		if !strings.Contains(migratedSchema, want) {
			t.Fatalf("column %q differs between a fresh and a repaired database:\n fresh: %s\nrepair: %s",
				column, want, migratedSchema)
		}
	}
}
