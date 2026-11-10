package main

import (
	"context"
	"database/sql"

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
