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
		// Scheduled runs (schedules.*): a saved instructions fired on a cron trigger as
		// a headless run. last_run_at / next_run_at are nullable unix millis;
		// next_run_at is the worker's due index and the one record of
		// whether the schedule fires.
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
		// Captured requests and acknowledgements survive definition deletion and
		// Runtime Run retirement. A late acknowledgement must not become a new dispatch.
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
		// A schedule has one recoverable occurrence at a time. Keeping this as a
		// partial uniqueness invariant prevents each later cron tick from adding
		// another pending firing while Run admission is temporarily unavailable.
		fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS idx_schedule_firings_schedule_pending
			ON schedule_firings(schedule_id) WHERE state = '%s'`, scheduleFiringPending.databaseValue()),
	})
}
