package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/cancellation"
)

// Cross-store transactions. The stores in this package each own their own
// table(s) but share one *sql.DB. Some use cases need a write-set that spans
// several stores atomically, so a mid-sequence failure must leave no partial
// state.
//
// A caller-supplied transaction is carried on the context: [RunInTx] begins it
// (or joins an outer one) and stashes it under txKey; every store statement
// runs through [conn], which uses that transaction when present and the shared
// pool otherwise. This is mandatory rather than an optimization: the pool runs
// at MaxOpenConns(1), so a store that opened its OWN BeginTx while a context
// transaction was live would deadlock — the single connection is already held.

// execer is the statement surface shared by *sql.DB and *sql.Tx, so a store
// method can run on either the pool or a caller's transaction unchanged.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// scanRow is the row surface shared by *sql.Row (QueryRow) and *sql.Rows (Query
// iteration), so one scan helper covers both a single Get and a List loop. Used
// by the goal / interrupt / agent-memory decoders.
type scanRow interface {
	Scan(dest ...any) error
}

type txKey struct{}

// conn returns the transaction carried on ctx by [RunInTx] if one is live,
// else the shared pool db. A store method MUST route through this (not the bare
// s.db) for any statement that should join a caller's cross-store transaction.
func Connection(ctx context.Context, db *sql.DB) execer {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return db
}

// RunInTx runs fn inside a single transaction exposed to fn's store calls via
// the returned context (they pick it up through [conn]). It commits on success
// and joins rollback on error or panic, so a multi-step write-set is atomic. A
// nested RunInTx joins the outer transaction (a single connection can't nest
// transactions, and the outer owns the commit), so a store method that wraps
// its own body in RunInTx is atomic standalone yet folds into a caller's
// transaction transparently.
func RunInTx(ctx context.Context, db *sql.DB, fn func(context.Context) error) error {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		if err := fn(ctx); err != nil {
			return errors.Join(err, cancellation.Err(ctx))
		}
		return nil
	}
	return runTransaction(ctx, db, fn)
}

// Acquisition and statements retain caller cancellation. The transaction has
// an owned lifetime so database/sql cannot race this boundary's rollback and
// discard its diagnostic. Workspace intents and schema admission deliberately
// enter here rather than joining an ambient write-set.
func runTransaction(ctx context.Context, db *sql.DB, fn func(context.Context) error) (err error) {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	connection, err := db.Conn(ctx)
	if err != nil {
		return errors.Join(err, cancellation.Err(ctx))
	}
	defer func() {
		if closeErr := connection.Close(); !errors.Is(closeErr, sql.ErrConnDone) {
			err = errors.Join(err, closeErr)
		}
	}()
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	tx, err := connection.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return errors.Join(err, discardConnection(connection), cancellation.Err(ctx))
	}
	defer func() {
		rollbackErr := rollbackTransaction(tx)
		if rollbackErr != nil {
			rollbackErr = errors.Join(rollbackErr, discardConnection(connection))
		}
		if recovered := recover(); recovered != nil {
			if rollbackErr != nil {
				panicErr, ok := recovered.(error)
				if !ok {
					panicErr = fmt.Errorf("sqlite: transaction panicked: %v", recovered)
				}
				panic(errors.Join(panicErr, rollbackErr, cancellation.Err(ctx)))
			}
			panic(recovered)
		}
		err = errors.Join(err, rollbackErr)
	}()
	if err = cancellation.Err(ctx); err == nil {
		err = fn(context.WithValue(ctx, txKey{}, tx))
	}
	err = errors.Join(err, cancellation.Err(ctx))
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(err, discardConnection(connection), cancellation.Err(ctx))
	}
	return nil
}

func rollbackTransaction(tx *sql.Tx) error {
	err := tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return err
}

// A failed transaction boundary cannot prove the connection is reusable.
// Raw's ErrBadConn contract discards it instead of returning it to the pool.
func discardConnection(connection *sql.Conn) error {
	err := connection.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return nil
	}
	return err
}
