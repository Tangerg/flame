package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// ScheduleStore is the SQLite persistence adapter for scheduled runs. The DB
// must have been opened via [Open] so the schedules table exists.
type ScheduleStore struct {
	db *sql.DB
}

// NewScheduleStore wires the given *sql.DB to the schedule persistence surface.
func NewScheduleStore(db *sql.DB) *ScheduleStore {
	return &ScheduleStore{db: db}
}

func (s *ScheduleStore) Insert(ctx context.Context, scheduled schedule.Schedule) error {
	if err := scheduled.Validate(); err != nil {
		return fmt.Errorf("sqlite: validate initial schedule: %w", err)
	}
	if scheduled.Revision() != exactint.First().Value() {
		return fmt.Errorf("sqlite: initial schedule revision is %d: %w", scheduled.Revision(), schedule.ErrRevisionConflict)
	}
	snapshot := scheduled.Snapshot()
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO schedules (id, title, instructions, cwd, provider, model, reasoning_effort, cron, enabled, last_run_at, next_run_at, created_at, revision)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.ID, snapshot.Title, snapshot.Instructions, snapshot.CWD,
		snapshot.ModelSelection.Provider(), snapshot.ModelSelection.Model(), snapshot.ModelSelection.ReasoningEffort(), snapshot.Cron,
		boolToInt(snapshot.Enabled), toMillis(snapshot.LastRunAt), toMillis(snapshot.NextRunAt), snapshot.CreatedAt.UnixMilli(), snapshot.Revision)
	if err != nil {
		return fmt.Errorf("sqlite: create schedule: %w", err)
	}
	return nil
}

func (s *ScheduleStore) Update(ctx context.Context, sc schedule.Schedule, expectedRevision uint64) (schedule.Schedule, error) {
	if err := sc.ValidateStored(); err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: validate schedule: %w", err)
	}
	if expectedRevision == 0 {
		return schedule.Schedule{}, schedule.ErrRevisionRequired
	}
	if err := exactint.Follows(expectedRevision, sc.Revision()); err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: replacement revision %d does not follow expected revision %d: %w", sc.Revision(), expectedRevision, schedule.ErrRevisionConflict)
	}
	snapshot := sc.Snapshot()
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE schedules
		 SET title = ?, instructions = ?, cwd = ?, provider = ?, model = ?, reasoning_effort = ?, cron = ?, enabled = ?, next_run_at = ?, revision = ?
		 WHERE id = ? AND revision = ?`,
		snapshot.Title, snapshot.Instructions, snapshot.CWD,
		snapshot.ModelSelection.Provider(), snapshot.ModelSelection.Model(), snapshot.ModelSelection.ReasoningEffort(), snapshot.Cron,
		boolToInt(snapshot.Enabled), toMillis(snapshot.NextRunAt), snapshot.Revision, snapshot.ID, expectedRevision)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: update schedule: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: inspect schedule update: %w", err)
	}
	if changed == 0 {
		if _, getErr := s.Get(ctx, sc.ID()); getErr != nil {
			return schedule.Schedule{}, getErr
		}
		return schedule.Schedule{}, schedule.ErrRevisionConflict
	}
	return s.Get(ctx, sc.ID())
}

func (s *ScheduleStore) Get(ctx context.Context, id string) (schedule.Schedule, error) {
	if err := schedule.ValidateID(id); err != nil {
		return schedule.Schedule{}, err
	}
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT id, title, instructions, cwd, provider, model, reasoning_effort, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules WHERE id = ?`, id)
	sc, err := scanSchedule(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: get schedule: %w", err)
	}
	return sc, nil
}

// ListPage returns schedules newest-created first, bounded by the query. after is
// the (creation time, id) position a previous page ended at; the id breaks ties,
// so two schedules created in the same nanosecond cannot be dropped or repeated
// across a page boundary.
func (s *ScheduleStore) ListPage(ctx context.Context, afterCreatedAt time.Time, afterID string, limit int) ([]schedule.Schedule, error) {
	if limit <= 0 {
		return nil, errors.New("sqlite: schedule page limit must be positive")
	}
	if afterID != "" {
		if err := schedule.ValidateID(afterID); err != nil {
			return nil, fmt.Errorf("sqlite: schedule page anchor: %w", err)
		}
	}
	query := `SELECT id, title, instructions, cwd, provider, model, reasoning_effort, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules`
	var args []any
	if !afterCreatedAt.IsZero() || afterID != "" {
		query += ` WHERE created_at < ? OR (created_at = ? AND id < ?)`
		afterMillis := afterCreatedAt.UnixMilli()
		args = append(args, afterMillis, afterMillis, afterID)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	query += ` LIMIT ?`
	args = append(args, limit)
	return s.query(ctx, "list schedules", query, args...)
}

func (s *ScheduleStore) Due(ctx context.Context, now time.Time, limit int) ([]schedule.Schedule, error) {
	if limit <= 0 {
		return nil, errors.New("sqlite: schedule due limit must be positive")
	}
	return s.query(ctx, "list due schedules",
		`SELECT id, title, instructions, cwd, provider, model, reasoning_effort, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules
		 WHERE enabled = 1 AND next_run_at > 0 AND next_run_at <= ?
		 ORDER BY next_run_at, id
		 LIMIT ?`, now.UnixMilli(), limit)
}

// Claim atomically advances a due schedule's cursor and materializes its
// immutable occurrence. The pending row is the durable work item a future
// worker dispatches after a process crash; cursor advancement therefore cannot
// produce either a duplicate accepted run or a silently lost occurrence.
// LastRunAt remains an accepted-Run fact and is intentionally not touched here.
func (s *ScheduleStore) Claim(ctx context.Context, claim schedule.Claim) (claimed bool, err error) {
	if err := claim.Validate(); err != nil {
		return false, fmt.Errorf("sqlite: invalid schedule claim: %w", err)
	}
	occurrence := claim.Occurrence()
	snapshot := occurrence.Snapshot()
	expectedRevision := claim.ExpectedRevision()
	revision, err := exactint.Restore(expectedRevision)
	if err != nil {
		return false, fmt.Errorf("sqlite: advance claimed schedule revision: %w", err)
	}
	next, err := revision.Next()
	if err != nil {
		return false, fmt.Errorf("sqlite: advance claimed schedule revision: %w", schedule.ErrRevisionExhausted)
	}
	execution := snapshot.Execution
	err = RunInTx(ctx, s.db, func(ctx context.Context) error {
		res, execContextErr := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE schedules SET next_run_at = ?, revision = ?
				 WHERE id = ? AND revision = ? AND next_run_at = ?
				   AND NOT EXISTS (
						SELECT 1 FROM schedule_firings
						 WHERE schedule_id = ? AND state = ?
				   )`,
			toMillis(snapshot.NextRunAt), next.Value(), snapshot.ScheduleID, expectedRevision,
			toMillis(snapshot.DueAt), snapshot.ScheduleID, scheduleFiringPending.databaseValue())
		if execContextErr != nil {
			return fmt.Errorf("sqlite: claim schedule occurrence: %w", execContextErr)
		}
		changed, execContextErr := res.RowsAffected()
		if execContextErr != nil {
			return fmt.Errorf("sqlite: inspect schedule occurrence claim: %w", execContextErr)
		}
		if changed == 0 {
			return nil
		}
		_, execContextErr = conn(ctx, s.db).ExecContext(ctx,
			`INSERT INTO schedule_firings(
				id, schedule_id, title, instructions, cwd, provider, model, reasoning_effort, cron,
				due_at, fired_at, next_run_at, session_id, run_id, state
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshot.ID, snapshot.ScheduleID, execution.Title, execution.Instructions,
			execution.CWD, execution.ModelSelection.Provider(), execution.ModelSelection.Model(), execution.ModelSelection.ReasoningEffort(), execution.Cron,
			toMillis(snapshot.DueAt), toMillis(snapshot.FiredAt), toMillis(snapshot.NextRunAt), snapshot.SessionID, snapshot.RunID,
			scheduleFiringPending.databaseValue())
		if execContextErr != nil {
			return fmt.Errorf("sqlite: persist schedule occurrence: %w", execContextErr)
		}
		claimed = true
		return nil
	})
	return claimed, err
}

// Pending lists durable occurrences whose Run opening has not committed. They
// carry a captured execution value, so later schedule edits or deletion cannot
// rewrite work that was already due.
func (s *ScheduleStore) Pending(ctx context.Context, limit int) ([]schedule.Occurrence, error) {
	if limit <= 0 {
		return nil, errors.New("sqlite: schedule pending limit must be positive")
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT id, schedule_id, title, instructions, cwd, provider, model, reasoning_effort, cron,
			due_at, fired_at, next_run_at, session_id, run_id
		 FROM schedule_firings WHERE state = ? ORDER BY due_at, id
		 LIMIT ?`, scheduleFiringPending.databaseValue(), limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list pending schedule occurrences: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var occurrences []schedule.Occurrence
	for rows.Next() {
		occurrence, err := scanOccurrence(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan pending schedule occurrence: %w", err)
		}
		occurrences = append(occurrences, occurrence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list pending schedule occurrences: %w", err)
	}
	return occurrences, nil
}

// Accept confirms the occurrence in the same transaction as its Run opening.
// Repeating the same confirmation is harmless; any other run id is a durable
// ownership violation rather than an invitation to create a duplicate run.
func (s *ScheduleStore) Accept(ctx context.Context, acceptance schedule.Acceptance) error {
	if err := acceptance.Validate(); err != nil {
		return fmt.Errorf("sqlite: invalid schedule acceptance: %w", err)
	}
	occurrenceID, runID := acceptance.OccurrenceID(), acceptance.RunID()
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		res, err := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE schedule_firings SET state = ? WHERE id = ? AND run_id = ? AND state = ?`,
			scheduleFiringAccepted.databaseValue(), occurrenceID, runID, scheduleFiringPending.databaseValue())
		if err != nil {
			return fmt.Errorf("sqlite: accept schedule occurrence: %w", err)
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect schedule occurrence acceptance: %w", err)
		}
		if changed != 0 {
			var scheduleID string
			var firedAt int64
			if scanErr := conn(ctx, s.db).QueryRowContext(ctx,
				`SELECT schedule_id, fired_at FROM schedule_firings WHERE id = ? AND run_id = ?`, occurrenceID, runID).Scan(&scheduleID, &firedAt); scanErr != nil {
				return fmt.Errorf("sqlite: load accepted schedule occurrence: %w", scanErr)
			}
			return s.advanceScheduleRunFact(ctx, scheduleID, firedAt)
		}
		var storedRunID, rawState string
		err = conn(ctx, s.db).QueryRowContext(ctx,
			`SELECT run_id, state FROM schedule_firings WHERE id = ?`, occurrenceID).Scan(&storedRunID, &rawState)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("sqlite: schedule occurrence not found")
		}
		if err != nil {
			return fmt.Errorf("sqlite: inspect schedule occurrence acceptance: %w", err)
		}
		state, restoreErr := restoreScheduleFiringState(rawState)
		if restoreErr != nil {
			return restoreErr
		}
		if storedRunID == runID && state == scheduleFiringAccepted {
			return nil
		}
		return errors.New("sqlite: schedule occurrence is owned by another run")
	})
}

// RecordRun moves only last_run_at; next_run_at is left as-is so a manual
// run-now never rewinds the cron cursor.
func (s *ScheduleStore) RecordRun(ctx context.Context, record schedule.RunRecord) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("sqlite: invalid schedule run record: %w", err)
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		return s.advanceScheduleRunFact(ctx, record.ScheduleID(), toMillis(record.RanAt()))
	})
}

func (s *ScheduleStore) advanceScheduleRunFact(ctx context.Context, id string, ranAtMillis int64) error {
	if err := schedule.ValidateID(id); err != nil {
		return err
	}
	result, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE schedules
		 SET last_run_at = MAX(last_run_at, ?), revision = revision + ?
		 WHERE id = ? AND revision < ?`,
		ranAtMillis, exactint.First().Value(), id, exactint.Maximum)
	if err != nil {
		return fmt.Errorf("sqlite: advance schedule run fact: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect advanced schedule run fact: %w", err)
	}
	if updated == 0 {
		return s.revisionAdvanceFailure(ctx, id)
	}
	return nil
}

func (s *ScheduleStore) revisionAdvanceFailure(ctx context.Context, id string) error {
	if err := schedule.ValidateID(id); err != nil {
		return err
	}
	var revision uint64
	err := conn(ctx, s.db).QueryRowContext(ctx, `SELECT revision FROM schedules WHERE id = ?`, id).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect schedule revision: %w", err)
	}
	if revision >= exactint.Maximum {
		return schedule.ErrRevisionExhausted
	}
	return errors.New("sqlite: schedule revision did not advance")
}

func (s *ScheduleStore) Delete(ctx context.Context, id string) (bool, error) {
	if err := schedule.ValidateID(id); err != nil {
		return false, err
	}
	result, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("sqlite: delete schedule: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect deleted schedule: %w", err)
	}
	return deleted > 0, nil
}

func (s *ScheduleStore) query(ctx context.Context, operation, q string, args ...any) ([]schedule.Schedule, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	defer func() { _ = rows.Close() }()
	var out []schedule.Schedule
	for rows.Next() {
		sc, err := scanSchedule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan schedule: %w", err)
		}
		out = append(out, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return out, nil
}

// scanSchedule decodes one row via the given Scan func (sql.Row or sql.Rows
// share the signature), converting the int-millis time columns back to
// time.Time (0 ⇒ zero time).
func scanSchedule(scan func(...any) error) (schedule.Schedule, error) {
	var snapshot schedule.Snapshot
	var provider, model, reasoningEffort string
	var enabled, lastMillis, nextMillis, createdMillis int64
	if err := scan(&snapshot.ID, &snapshot.Title, &snapshot.Instructions, &snapshot.CWD, &provider, &model, &reasoningEffort, &snapshot.Cron,
		&enabled, &lastMillis, &nextMillis, &createdMillis, &snapshot.Revision); err != nil {
		return schedule.Schedule{}, err
	}
	selection, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: decode schedule model selection: %w", err)
	}
	snapshot.ModelSelection = selection
	snapshot.Enabled = enabled != 0
	snapshot.LastRunAt = fromMillis(lastMillis)
	snapshot.NextRunAt = fromMillis(nextMillis)
	snapshot.CreatedAt = time.UnixMilli(createdMillis).UTC()
	scheduled, err := schedule.Restore(snapshot)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: restore schedule: %w", err)
	}
	return scheduled, nil
}

func scanOccurrence(scan func(...any) error) (schedule.Occurrence, error) {
	var snapshot schedule.OccurrenceSnapshot
	var title, instructions, cwd, provider, model, reasoningEffort, cron string
	var dueAt, firedAt, nextRunAt int64
	if err := scan(&snapshot.ID, &snapshot.ScheduleID, &title, &instructions,
		&cwd, &provider, &model, &reasoningEffort, &cron,
		&dueAt, &firedAt, &nextRunAt, &snapshot.SessionID, &snapshot.RunID); err != nil {
		return schedule.Occurrence{}, err
	}
	selection, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return schedule.Occurrence{}, fmt.Errorf("decode schedule occurrence model selection: %w", err)
	}
	snapshot.Execution = schedule.ExecutionSnapshot{
		Title: title, Instructions: instructions, CWD: cwd,
		ModelSelection: selection, Cron: cron,
	}
	snapshot.DueAt = fromMillis(dueAt)
	snapshot.FiredAt = fromMillis(firedAt)
	snapshot.NextRunAt = fromMillis(nextRunAt)
	occurrence, err := schedule.RestoreOccurrence(snapshot)
	if err != nil {
		return schedule.Occurrence{}, fmt.Errorf("decode schedule occurrence: %w", err)
	}
	return occurrence, nil
}
