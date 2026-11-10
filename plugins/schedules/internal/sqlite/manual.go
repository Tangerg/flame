package sqlite

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"

	framesqlite "github.com/Tangerg/frame/sqlite"

	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

var manualSchema = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS manual_firings (run_id TEXT PRIMARY KEY, captured_request_json TEXT NOT NULL CHECK(json_valid(captured_request_json)) CHECK(json_type(captured_request_json,'$.CommandID') IS 'text' AND length(json_extract(captured_request_json,'$.CommandID')) > 0), state TEXT NOT NULL CHECK(state IN ('%s','%s')))`, scheduleFiringPending.databaseValue(), scheduleFiringAccepted.databaseValue())

func (s *ScheduleStore) QueueManual(ctx context.Context, request schedule.RunRequest) error {
	return framesqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
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
		_, err = framesqlite.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO manual_firings(run_id,captured_request_json,state) VALUES(?,?,?)`, request.RunID(), string(encoded), scheduleFiringPending.databaseValue())
		return err
	})
}
func (s *ScheduleStore) PendingManual(ctx context.Context, afterID string, limit int) ([]schedule.RunRequest, error) {
	var result []schedule.RunRequest
	err := framesqlite.RunReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		if limit <= 0 || limit > schedule.MaximumDispatchBatch {

			return errors.New("schedule: invalid manual dispatch limit")
		}
		rows, err := framesqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT run_id,captured_request_json FROM manual_firings WHERE state=? AND run_id>? ORDER BY run_id LIMIT ?`, scheduleFiringPending.databaseValue(), afterID, limit)
		if err != nil {

			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, encoded string
			if err := rows.Scan(&id, &encoded); err != nil {

				return err
			}
			request, err := restoreManual(id, encoded)
			if err != nil {

				return err
			}
			result = append(result, request)
		}
		return rows.Err()
	})
	return result, err
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
	return framesqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		var rawID string
		err := framesqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT id FROM schedule_firings WHERE run_id=?`, id).Scan(&rawID)
		if err == nil {
			occurrenceID, err := schedule.ParseAcceptance(rawID, id)
			if err != nil {
				return err
			}
			return s.accept(ctx, occurrenceID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var encoded, state string
		if err := framesqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT captured_request_json,state FROM manual_firings WHERE run_id=?`, id).Scan(&encoded, &state); err != nil {
			return fmt.Errorf("schedule: find manual dispatch: %w", err)
		}
		request, err := restoreManual(id, encoded)
		if err != nil {
			return err
		}
		firingState, err := restoreScheduleFiringState(state)
		if err != nil {
			return err
		}
		if firingState == scheduleFiringAccepted {
			return nil
		}
		record, _ := request.ManualRecord()

		if _, err := s.recordAcceptedRun(ctx, record.ScheduleID(), record.RanAt()); err != nil {
			return err
		}
		_, err = framesqlite.Connection(ctx, s.db).ExecContext(ctx, `UPDATE manual_firings SET state=? WHERE run_id=?`, scheduleFiringAccepted.databaseValue(), id)
		return err
	})
}
