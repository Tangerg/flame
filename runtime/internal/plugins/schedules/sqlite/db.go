package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/exactint"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	firstExactInteger := exactint.First().Value()
	return sqlitedb.OpenSchema(ctx, path, []string{managedmcp.CommandsSchema, manualSchema,
		`CREATE TABLE IF NOT EXISTS runtime_target (id INTEGER PRIMARY KEY CHECK(id=1), namespace TEXT NOT NULL)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS schedules (
			id          TEXT    PRIMARY KEY,
			title       TEXT    NOT NULL DEFAULT '',
			instructions      TEXT    NOT NULL,
			cwd         TEXT    NOT NULL DEFAULT '',
			provider    TEXT    NOT NULL DEFAULT '',
			model       TEXT    NOT NULL DEFAULT '',
			reasoning_effort TEXT NOT NULL DEFAULT '',
			cron        TEXT    NOT NULL,
			last_run_at INTEGER,
			next_run_at INTEGER,
			created_at  INTEGER NOT NULL,
			revision    INTEGER NOT NULL DEFAULT %d CHECK (revision BETWEEN %d AND %d)
		)`, firstExactInteger, firstExactInteger, exactint.Maximum),
		`CREATE INDEX IF NOT EXISTS idx_schedules_due
			ON schedules(next_run_at)`,
		// Accepted rows outlive Runtime Runs so acknowledgement replay cannot dispatch again.
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS schedule_firings (
			id          TEXT    PRIMARY KEY,
			schedule_id TEXT    NOT NULL,
			title       TEXT    NOT NULL DEFAULT '',
			instructions      TEXT    NOT NULL,
			cwd         TEXT    NOT NULL DEFAULT '',
			provider    TEXT    NOT NULL DEFAULT '',
			model       TEXT    NOT NULL DEFAULT '',
			reasoning_effort TEXT NOT NULL DEFAULT '',
			due_at      INTEGER NOT NULL,
			fired_at    INTEGER NOT NULL,
			next_run_at INTEGER NOT NULL,
			session_id  TEXT    NOT NULL UNIQUE,
			run_id      TEXT    NOT NULL UNIQUE,
			state       TEXT    NOT NULL CHECK(state IN ('%s', '%s')),
			CHECK (id = schedule_id || ':' || CAST(due_at AS TEXT))
		)`, scheduleFiringPending.databaseValue(), scheduleFiringAccepted.databaseValue()),
		`CREATE INDEX IF NOT EXISTS idx_schedule_firings_pending
			ON schedule_firings(state, due_at, id)`,
		// One pending occurrence prevents unavailable Run admission from accumulating a new firing on each cron tick.
		fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS idx_schedule_firings_schedule_pending
			ON schedule_firings(schedule_id) WHERE state = '%s'`, scheduleFiringPending.databaseValue()),
	})
}
