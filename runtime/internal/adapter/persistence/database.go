package persistence

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"

	"github.com/Tangerg/frame"
	"github.com/Tangerg/frame/sqlite"
)

// A store only borrows capabilities issued by its database owner. The boundary
// has a separate implementation for independently owned test databases.
type databaseBoundary interface {
	identity() any
	read(context.Context, func(sqlite.Reader) error) error
	write(context.Context, func(context.Context, sqlite.Executor) error) error
}

type database struct{ boundary databaseBoundary }

type frameDatabase struct {
	store     *sqlite.Store
	owner     *sqlite.Owner
	admission *frame.Admission
}

func newDatabase(store *sqlite.Store, admission *frame.Admission) (*database, error) {
	owner, err := store.Owner("flame")
	if err != nil {
		return nil, err
	}
	return &database{boundary: &frameDatabase{store: store, owner: owner, admission: admission}}, nil
}
func (d *frameDatabase) identity() any { return d.store }
func (d *frameDatabase) read(ctx context.Context, read func(sqlite.Reader) error) error {
	return d.store.Read(ctx, read)
}
func (d *frameDatabase) write(ctx context.Context, write func(context.Context, sqlite.Executor) error) error {
	if d.admission.CheckAdmitted(ctx) == nil {
		return errors.New("persistence: admitted mutation requires its original transaction capability")
	}
	return d.owner.Update(ctx, write)
}

type databaseScopeKey struct{ owner any }
type databaseScope struct {
	reader sqlite.Reader
	writer sqlite.Executor
	active atomic.Bool
}

func (d *database) scope(ctx context.Context) (*databaseScope, bool) {
	scope, found := ctx.Value(databaseScopeKey{d.boundary.identity()}).(*databaseScope)
	return scope, found
}

func (d *database) withReader(ctx context.Context, reader sqlite.Reader, read func(context.Context) error) error {
	return d.withScope(ctx, &databaseScope{reader: reader}, read)
}
func (d *database) withExecutor(ctx context.Context, executor sqlite.Executor, write func(context.Context) error) error {
	return d.withScope(ctx, &databaseScope{reader: executor, writer: executor}, write)
}
func (d *database) withScope(ctx context.Context, scope *databaseScope, work func(context.Context) error) error {
	scope.active.Store(true)
	defer scope.active.Store(false)
	return work(context.WithValue(ctx, databaseScopeKey{d.boundary.identity()}, scope))
}

func (d *database) read(ctx context.Context, read func(context.Context) error) error {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	if original, found := d.scope(ctx); found {
		if !original.active.Load() {
			return sql.ErrTxDone
		}
		return d.withReader(ctx, original.reader, read)
	}
	return d.boundary.read(ctx, func(reader sqlite.Reader) error { return d.withReader(ctx, reader, read) })
}

func (d *database) write(ctx context.Context, write func(context.Context) error) error {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return err
	}
	if original, found := d.scope(ctx); found {
		if !original.active.Load() {
			return sql.ErrTxDone
		}
		if original.writer == nil {
			return errors.New("persistence: write cannot enter a read snapshot")
		}
		return write(ctx)
	}
	return d.boundary.write(ctx, func(ctx context.Context, executor sqlite.Executor) error { return d.withExecutor(ctx, executor, write) })
}

func (d *database) independent(ctx context.Context, write func(context.Context) error) error {
	if _, active := d.scope(ctx); active {
		return errors.New("persistence: independent write cannot enter an active transaction")
	}
	return d.write(ctx, write)
}

// SQL is private to the lexical read/write methods above. Frame seals and joins
// the actual capabilities and cursors; this context is never a database handle.
func (d *database) connection(ctx context.Context) sqlite.Executor {
	scope, found := d.scope(ctx)
	if !found || !scope.active.Load() {
		panic("persistence: SQL requires an active database scope")
	}
	if scope.writer != nil {
		return scope.writer
	}
	return readExecutor{Reader: scope.reader}
}

type readExecutor struct{ sqlite.Reader }

func (readExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("persistence: write cannot enter a read snapshot")
}

func scopedValue[T any](ctx context.Context, scope func(context.Context, func(context.Context) error) error, read func(context.Context) (T, error)) (value T, err error) {
	err = scope(ctx, func(ctx context.Context) error { value, err = read(ctx); return err })
	return
}
func scopedPair[A, B any](ctx context.Context, scope func(context.Context, func(context.Context) error) error, read func(context.Context) (A, B, error)) (a A, b B, err error) {
	err = scope(ctx, func(ctx context.Context) error { a, b, err = read(ctx); return err })
	return
}
func scopedTriple[A, B, C any](ctx context.Context, scope func(context.Context, func(context.Context) error) error, read func(context.Context) (A, B, C, error)) (a A, b B, c C, err error) {
	err = scope(ctx, func(ctx context.Context) error { a, b, c, err = read(ctx); return err })
	return
}
