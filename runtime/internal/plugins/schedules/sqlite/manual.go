package sqlite

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"

	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
)

const manualSchema = `CREATE TABLE IF NOT EXISTS manual_firings (run_id TEXT PRIMARY KEY, request TEXT NOT NULL CHECK(json_valid(request)), state TEXT NOT NULL CHECK(state IN ('pending','accepted')))`

func (s *ScheduleStore) QueueManual(ctx context.Context, request schedule.RunRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if _, manual := request.ManualRecord(); !manual {
		return errors.New("schedule: manual queue requires a manual request")
	}
	encoded, err := json.Marshal(request.Snapshot())
	if err != nil {
		return err
	}
	_, err = sqlitedb.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO manual_firings(run_id,request,state) VALUES(?,?,'pending')`, request.RunID(), string(encoded))
	return err
}
func (s *ScheduleStore) PendingManual(ctx context.Context, afterID string, limit int) ([]schedule.RunRequest, error) {
	if limit <= 0 || limit > schedule.MaximumDispatchBatch {
		return nil, errors.New("schedule: invalid manual dispatch limit")
	}
	rows, err := sqlitedb.Connection(ctx, s.db).QueryContext(ctx, `SELECT run_id,request FROM manual_firings WHERE state='pending' AND run_id>? ORDER BY run_id LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []schedule.RunRequest
	for rows.Next() {
		var id, encoded string
		if err := rows.Scan(&id, &encoded); err != nil {
			return nil, err
		}
		request, err := restoreManual(id, encoded)
		if err != nil {
			return nil, err
		}
		result = append(result, request)
	}
	return result, rows.Err()
}
func restoreManual(id, encoded string) (schedule.RunRequest, error) {
	var snapshot schedule.RunRequestSnapshot
	if err := json.Unmarshal([]byte(encoded), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return schedule.RunRequest{}, err
	}
	request, err := schedule.RestoreRunRequest(snapshot)
	if err != nil {
		return schedule.RunRequest{}, err
	}
	if request.RunID() != id {
		return schedule.RunRequest{}, errors.New("schedule: manual dispatch identity disagrees with stored request")
	}
	if _, manual := request.ManualRecord(); !manual {
		return schedule.RunRequest{}, errors.New("schedule: cron request in manual queue")
	}
	return request, nil
}
func (s *ScheduleStore) AcceptRun(ctx context.Context, id string) error {
	return sqlitedb.RunInTx(ctx, s.db, func(ctx context.Context) error {
		var rawID string
		err := sqlitedb.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT id FROM schedule_firings WHERE run_id=?`, id).Scan(&rawID)
		if err == nil {
			occurrenceID, err := schedule.ParseAcceptance(rawID, id)
			if err != nil {
				return err
			}
			return s.Accept(ctx, occurrenceID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var encoded, state string
		if err := sqlitedb.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT request,state FROM manual_firings WHERE run_id=?`, id).Scan(&encoded, &state); err != nil {
			return fmt.Errorf("schedule: find manual dispatch: %w", err)
		}
		request, err := restoreManual(id, encoded)
		if err != nil {
			return err
		}
		switch state {
		case "accepted":
			return nil
		case "pending":
		default:
			return errors.New("schedule: invalid manual dispatch state")
		}
		record, _ := request.ManualRecord()

		if _, err := s.recordAcceptedRun(ctx, record.ScheduleID(), record.RanAt()); err != nil {
			return err
		}
		_, err = sqlitedb.Connection(ctx, s.db).ExecContext(ctx, `UPDATE manual_firings SET state='accepted' WHERE run_id=?`, id)
		return err
	})
}
