package main

import (
	"context"

	framesqlite "github.com/Tangerg/frame/sqlite"
)

func scanDatabase(ctx context.Context, db framesqlite.Database, query string, args []any, values ...any) error {
	return db.Read(ctx, func(reader framesqlite.Reader) error {
		return reader.QueryRowContext(ctx, query, args...).Scan(values...)
	})
}
