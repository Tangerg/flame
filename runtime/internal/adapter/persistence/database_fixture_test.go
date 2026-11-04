package persistence

import (
	"context"
	"database/sql"

	"github.com/Tangerg/frame/sqlite"
)

// Snapshot and fault fixtures own independent databases, outside the Host file.
type independentDatabase struct{ db *sql.DB }

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
	case *sql.DB:
		return &database{boundary: independentDatabase{source}}
	default:
		panic("persistence fixture requires an owned database")
	}
}

func rawFixtureDatabase(db *database) *sql.DB {
	owned, ok := db.boundary.(independentDatabase)
	if !ok {
		panic("fixture SQL requires an independently owned database")
	}
	return owned.db
}
