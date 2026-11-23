package sqlite

import (
	"context"
	"database/sql"
	"errors"

	framesqlite "github.com/Tangerg/frame/sqlite"
)

const RuntimeTargetSchema = `CREATE TABLE IF NOT EXISTS runtime_target (id INTEGER PRIMARY KEY CHECK(id=1), namespace TEXT NOT NULL)`

var ErrRuntimeTargetConflict = errors.New("managed plugin: private data belongs to another Runtime store")

func CheckRuntimeTarget(ctx context.Context, db framesqlite.Database, namespace string) error {
	if namespace == "" {
		return errors.New("managed plugin: Runtime store namespace is required")
	}
	var bound string
	err := db.Read(ctx, func(reader framesqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT namespace FROM runtime_target WHERE id=1`).Scan(&bound)
	})
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

func BindRuntimeTarget(ctx context.Context, db framesqlite.Database, namespace string) error {
	return db.RunInTx(ctx, func(ctx context.Context) error {
		if err := CheckRuntimeTarget(ctx, db, namespace); err != nil {
			return err
		}
		_, err := db.Executor(ctx).ExecContext(ctx, `INSERT INTO runtime_target(id,namespace) VALUES(1,?) ON CONFLICT(id) DO NOTHING`, namespace)
		return err
	})
}
