package sqlite

import (
	"context"
	"database/sql"
	"errors"

	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	sqlitebase "github.com/Tangerg/flame/packages/go-sdk/storage/sqlite"
)

func (s *ScheduleStore) CapturedRun(ctx context.Context, id string) (schedule.RunRequest, error) {
	occurrence, err := scanOccurrence(sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT id,title,instructions,cwd,provider,model,reasoning_effort,fired_at,next_run_at,session_id,run_id,invocation_json FROM schedule_firings WHERE run_id=?`, id).Scan)
	if err == nil {
		return occurrence.RunRequest(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return schedule.RunRequest{}, err
	}
	var encoded string
	if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT captured_request_json FROM manual_firings WHERE run_id=?`, id).Scan(&encoded); err != nil {
		return schedule.RunRequest{}, err
	}
	return restoreManual(id, encoded)
}
