package sqlite

import (
	"context"
	"database/sql"
	"testing"

	framesqlite "github.com/Tangerg/frame/sqlite"
)

func scanDatabase(ctx context.Context, db *framesqlite.Database, query string, args []any, values ...any) error {
	return db.Read(ctx, func(reader framesqlite.Reader) error {
		return reader.QueryRowContext(ctx, query, args...).Scan(values...)
	})
}

func execDatabase(ctx context.Context, db *framesqlite.Database, statement string, args ...any) (sql.Result, error) {
	var result sql.Result
	err := framesqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		var err error
		result, err = framesqlite.Connection(ctx, db).ExecContext(ctx, statement, args...)
		return err
	})
	return result, err
}

// Historical and damaged schemas must be supplied externally; a live owner
// correctly refuses schema changes through its transaction capabilities.
func alterClosedDatabase(t *testing.T, db *framesqlite.Database, path string, statements ...string) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, statement := range statements {
		if _, err := raw.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
}
