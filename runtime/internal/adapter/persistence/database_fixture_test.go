package persistence

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/Tangerg/frame/sqlite"
)

// Snapshot and fault fixtures own independent databases, outside the Host file.
type independentDatabase struct{ db *sqlite.Database }

func (d independentDatabase) identity() any { return d.db }
func (d independentDatabase) read(ctx context.Context, read func(sqlite.Reader) error) error {
	return sqlite.RunReadSnapshot(ctx, d.db, func(ctx context.Context) error { return read(sqlite.Connection(ctx, d.db)) })
}
func (d independentDatabase) write(ctx context.Context, write func(context.Context, sqlite.Executor) error) error {
	return sqlite.RunInTx(ctx, d.db, func(ctx context.Context) error { return write(ctx, sqlite.Connection(ctx, d.db)) })
}
func databaseFor(source any) *database {
	switch source := source.(type) {
	case *database:
		return source
	case *sqlite.Database:
		return &database{boundary: independentDatabase{source}}
	default:
		panic("persistence fixture requires an owned database")
	}
}

func fixtureDatabase(db *database) *sqlite.Database {
	owned, ok := db.boundary.(independentDatabase)
	if !ok {
		panic("fixture SQL requires an independently owned database")
	}
	return owned.db
}

func openDatabase(ctx context.Context, path string) (*sqlite.Database, error) {
	return sqlite.OpenSchema(ctx, path, currentSchema())
}

func execFixture(ctx context.Context, db *sqlite.Database, statement string, args ...any) (result sql.Result, err error) {
	err = sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		result, err = sqlite.Connection(ctx, db).ExecContext(ctx, statement, args...)
		return err
	})
	return
}

func scanFixture(ctx context.Context, db *sqlite.Database, query string, args []any, destinations ...any) error {
	return db.Read(ctx, func(reader sqlite.Reader) error {
		return reader.QueryRowContext(ctx, query, args...).Scan(destinations...)
	})
}

func fixtureColumns(t testing.TB, db *sqlite.Database, table string) (columns []string) {
	t.Helper()
	if err := db.Read(t.Context(), func(reader sqlite.Reader) error {
		rows, err := reader.QueryContext(t.Context(), `SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			columns = append(columns, name)
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return
}

// Corruption is introduced only after the original owner has released the file.
// Reopening creates a new owner that must reject the untrusted stored values.
func corruptFixture(t testing.TB, db *sqlite.Database, corrupt func(*sql.Conn) error) *sqlite.Database {
	t.Helper()
	var path string
	if err := scanFixture(t.Context(), db, `SELECT file FROM pragma_database_list WHERE name='main'`, nil, &path); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := raw.Conn(t.Context())
	if err != nil {
		t.Fatal(errors.Join(err, raw.Close()))
	}
	if err := errors.Join(corrupt(connection), connection.Close(), raw.Close()); err != nil {
		t.Fatal(err)
	}
	reopened, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	return reopened
}
