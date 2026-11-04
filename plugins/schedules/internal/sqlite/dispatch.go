package sqlite

import (
	"context"
	"database/sql"
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"

	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

func (s *ScheduleStore) CapturedRun(ctx context.Context, id string) (schedule.RunRequest, error) {
	occurrence, err := scanOccurrence(pluginstorage.Connection(ctx, s.db).QueryRowContext(ctx,
		`SELECT id,title,instructions,cwd,provider,model,reasoning_effort,fired_at,next_run_at,session_id,run_id,invocation_json FROM schedule_firings WHERE run_id=?`, id).Scan)
	if err == nil {
		return occurrence.RunRequest(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return schedule.RunRequest{}, err
	}
	var encoded string
	if err := pluginstorage.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT captured_request_json FROM manual_firings WHERE run_id=?`, id).Scan(&encoded); err != nil {
		return schedule.RunRequest{}, err
	}
	return restoreManual(id, encoded)
}
