package sqlite

import (
	"context"
	"database/sql"
	"errors"

	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

func (s *ScheduleStore) CapturedRun(ctx context.Context, id string) (schedule.RunRequest, error) {
	var result schedule.RunRequest
	err := s.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		occurrence, err := scanOccurrence(s.db.Executor(ctx).QueryRowContext(ctx,
			`SELECT id,title,instructions,cwd,provider,model,reasoning_effort,fired_at,next_run_at,session_id,run_id,invocation_json FROM schedule_firings WHERE run_id=?`, id).Scan)
		if err == nil {
			result = occurrence.RunRequest()
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var encoded string
		if err := s.db.Executor(ctx).QueryRowContext(ctx, `SELECT captured_request_json FROM manual_firings WHERE run_id=?`, id).Scan(&encoded); err != nil {
			return err
		}
		var scopeErr error
		result, scopeErr = restoreManual(id, encoded)
		return scopeErr
	})
	return result, err
}
