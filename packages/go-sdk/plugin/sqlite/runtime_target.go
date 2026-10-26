package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
)

// RuntimeTargetSchema binds a managed plugin's private database to exactly one
// Runtime store namespace through a singleton row, so durable state can never
// serve a Runtime it was not captured for.
const RuntimeTargetSchema = `CREATE TABLE IF NOT EXISTS runtime_target (id INTEGER PRIMARY KEY CHECK(id=1), namespace TEXT NOT NULL)`

// ErrRuntimeTargetConflict reports private data already bound to a different
// Runtime store namespace.
var ErrRuntimeTargetConflict = errors.New("managed plugin: private data belongs to another Runtime store")

// CheckRuntimeTarget accepts an unbound database or one already bound to
// namespace; any other binding is ErrRuntimeTargetConflict. It reads through
// the ambient transaction when one is open.
func CheckRuntimeTarget(ctx context.Context, db *sql.DB, namespace string) error {
	if namespace == "" {
		return errors.New("managed plugin: Runtime store namespace is required")
	}
	var bound string
	err := sqlite.Connection(ctx, db).QueryRowContext(ctx, `SELECT namespace FROM runtime_target WHERE id=1`).Scan(&bound)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if bound != namespace {
		return ErrRuntimeTargetConflict
	}
	return nil
}

// BindRuntimeTarget binds the database to namespace, idempotent for the same
// namespace and ErrRuntimeTargetConflict for any other.
func BindRuntimeTarget(ctx context.Context, db *sql.DB, namespace string) error {
	return sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		if err := CheckRuntimeTarget(ctx, db, namespace); err != nil {
			return err
		}
		_, err := sqlite.Connection(ctx, db).ExecContext(ctx, `INSERT INTO runtime_target(id,namespace) VALUES(1,?) ON CONFLICT(id) DO NOTHING`, namespace)
		return err
	})
}
