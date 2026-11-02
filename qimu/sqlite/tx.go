package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/frame/cancellation"
	framesqlite "github.com/Tangerg/frame/sqlite"
)

type borrowedTransaction struct{ database *sql.DB }

func borrowed(ctx context.Context, db *sql.DB) Executor {
	executor, _ := ctx.Value(borrowedTransaction{database: db}).(Executor)
	return executor
}

// Owner commits lend their executor until the callback returns. Ordinary
// transactions retain Frame's isolation and cancellation semantics.
func Connection(ctx context.Context, db *sql.DB) Executor {
	if executor := borrowed(ctx, db); executor != nil {
		return executor
	}
	return framesqlite.Connection(ctx, db)
}

func RunInTx(ctx context.Context, db *sql.DB, apply func(context.Context) error) error {
	if borrowed(ctx, db) == nil {
		return framesqlite.RunInTx(ctx, db, apply)
	}
	return useBorrowed(ctx, apply)
}

func RunReadSnapshot(ctx context.Context, db *sql.DB, read func(context.Context) error) error {
	if borrowed(ctx, db) == nil {
		return framesqlite.RunReadSnapshot(ctx, db, read)
	}
	return useBorrowed(ctx, read)
}

func useBorrowed(ctx context.Context, apply func(context.Context) error) error {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	err := apply(ctx)
	if canceled := cancellation.Err(ctx); canceled != nil {
		return errors.Join(err, canceled)
	}
	return err
}

func RunIndependentTx(ctx context.Context, db *sql.DB, apply func(context.Context) error) error {
	if borrowed(ctx, db) != nil {
		return errors.Join(errors.New("sqlite: independent transaction cannot enter an owner commit"), cancellation.Err(ctx))
	}
	return framesqlite.RunIndependentTx(ctx, db, apply)
}
