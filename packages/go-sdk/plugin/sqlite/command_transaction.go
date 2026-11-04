package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"

	framesqlite "github.com/Tangerg/frame/sqlite"
)

type commandTransactionKey struct{}

type commandTransaction struct {
	database    *sql.DB
	transaction *sql.Tx
	active      atomic.Bool
}

func (s *commandTransaction) validate(db *sql.DB) error {
	if !s.active.Load() {
		return errors.New("managed plugin: command transaction scope has ended")
	}
	if s.database != db {
		return errors.New("managed plugin: command transaction belongs to another database")
	}
	return nil
}

// Connection borrows the original private command write-set. Outside a command,
// Frame remains the owner of standalone transaction scopes. The returned value
// exposes SQL work, never authority to settle the command's transaction.
func Connection(ctx context.Context, db *sql.DB) commandConnection {
	if scope, ok := ctx.Value(commandTransactionKey{}).(*commandTransaction); ok {
		return commandConnection{scope: scope, database: db}
	}
	return commandConnection{original: framesqlite.Connection(ctx, db)}
}

type commandConnection struct {
	scope    *commandTransaction
	database *sql.DB
	original framesqlite.Executor
}

type commandRow interface {
	Scan(...any) error
	Err() error
}

func (c commandConnection) ExecContext(ctx context.Context, statement string, args ...any) (sql.Result, error) {
	if c.scope != nil {
		if err := c.scope.validate(c.database); err != nil {
			return nil, err
		}
		return c.scope.transaction.ExecContext(ctx, statement, args...)
	}
	return c.original.ExecContext(ctx, statement, args...)
}

func (c commandConnection) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	if c.scope != nil {
		if err := c.scope.validate(c.database); err != nil {
			return nil, err
		}
		return c.scope.transaction.QueryContext(ctx, statement, args...)
	}
	return c.original.QueryContext(ctx, statement, args...)
}

func (c commandConnection) QueryRowContext(ctx context.Context, statement string, args ...any) commandRow {
	if c.scope != nil {
		if err := c.scope.validate(c.database); err != nil {
			return failedCommandRow{err: err}
		}
		return c.scope.transaction.QueryRowContext(ctx, statement, args...)
	}
	return c.original.QueryRowContext(ctx, statement, args...)
}

type failedCommandRow struct{ err error }

func (r failedCommandRow) Scan(...any) error { return r.err }
func (r failedCommandRow) Err() error        { return r.err }

// WithinTransaction joins a command's existing write-set. A repository mutation
// cannot commit independently of the reply that records that same mutation.
func WithinTransaction(ctx context.Context, db *sql.DB, apply func(context.Context) error) error {
	scope, ok := ctx.Value(commandTransactionKey{}).(*commandTransaction)
	if !ok {
		return framesqlite.RunInTx(ctx, db, apply)
	}
	if err := scope.validate(db); err != nil {
		return err
	}
	if apply == nil {
		return errors.New("managed plugin: repository mutation is required")
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	return errors.Join(apply(ctx), ctx.Err(), context.Cause(ctx))
}

// A classified business refusal must roll back its SQL changes and retain its
// reply atomically. Frame's scoped executor deliberately forbids savepoints, so
// this receipt owner uses its own private database transaction, never a Frame
// Store connection. An uncertain commit cannot publish the prepared reply.
func runCommandTransaction(ctx context.Context, db *sql.DB, apply func(context.Context, *sql.Tx) error) (returned error) {
	if _, nested := ctx.Value(commandTransactionKey{}).(*commandTransaction); nested {
		return errors.New("managed plugin: a receipt command cannot enter another command transaction")
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	connection, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	scope := &commandTransaction{database: db, transaction: tx}
	scope.active.Store(true)
	defer func() {
		scope.active.Store(false)
		rollback := tx.Rollback()
		if rollback != nil && !errors.Is(rollback, sql.ErrTxDone) {
			returned = errors.Join(returned, rollback, discardCommandConnection(connection))
		}
	}()
	if err := apply(context.WithValue(ctx, commandTransactionKey{}, scope), tx); err != nil {
		return err
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	scope.active.Store(false)
	if err := tx.Commit(); err != nil {
		return errors.Join(ErrCommandUnconfirmed, err, discardCommandConnection(connection))
	}
	return nil
}

// database/sql marks failed Commit and Rollback calls terminal even if the
// driver left the original write-set open. That connection cannot be reused.
func discardCommandConnection(connection *sql.Conn) error {
	err := connection.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return nil
	}
	return err
}
