package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/qimu/sqlite"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	firstExactInteger := exactint.First().Value()
	return sqlite.OpenSchema(ctx, path, []string{pluginstorage.CommandsSchema, manualSchema,
		pluginstorage.RuntimeTargetSchema,

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
			invocation_json TEXT NOT NULL CHECK(json_valid(invocation_json)),
			state       TEXT    NOT NULL CHECK(state IN ('%s', '%s')),
			CHECK (id = schedule_id || ':' || CAST(due_at AS TEXT))
		)`, scheduleFiringPending.databaseValue(), scheduleFiringAccepted.databaseValue()),
		`CREATE TRIGGER IF NOT EXISTS immutable_schedule_invocation BEFORE UPDATE OF invocation_json ON schedule_firings
		 BEGIN SELECT RAISE(ABORT,'captured invocation is immutable'); END`,
		`CREATE TRIGGER IF NOT EXISTS immutable_manual_request BEFORE UPDATE OF captured_request_json ON manual_firings
		 BEGIN SELECT RAISE(ABORT,'captured request is immutable'); END`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_firings_pending
			ON schedule_firings(state, due_at, id)`,
		// One pending occurrence prevents unavailable Run admission from accumulating a new firing on each cron tick.
		fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS idx_schedule_firings_schedule_pending
			ON schedule_firings(schedule_id) WHERE state = '%s'`, scheduleFiringPending.databaseValue()),
	})
}
