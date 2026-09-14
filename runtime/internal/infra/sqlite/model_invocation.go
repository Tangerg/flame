package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

type modelInvocationState string

const (
	modelInvocationStarted   modelInvocationState = "started"
	modelInvocationCompleted modelInvocationState = "completed"
	modelInvocationFailed    modelInvocationState = "failed"
	modelInvocationUnknown   modelInvocationState = "unknown"
)

func (m modelInvocationState) databaseValue() string { return string(m) }

// ModelInvocationStore is the SQLite operational journal for provider-call
// attempts. Semantic messages and aggregate accounting stay in history_items
// and runs. Attempt timing, outcomes, and per-call usage remain available until
// the owning Run is deleted.
type ModelInvocationStore struct{ db *sql.DB }

func NewModelInvocationStore(db *sql.DB) *ModelInvocationStore {
	return &ModelInvocationStore{db: db}
}

// ListStartedModelInvocations yields every provider attempt still owned by a
// pre-crash process. The callback keeps recovery application types out of the
// SQLite package while preserving an exact boot snapshot for its write-set.
func (m *ModelInvocationStore) ListStartedModelInvocations(
	ctx context.Context,
	yield func(sessionID, runID, segmentID, callID string, startedAt time.Time) error,
) error {
	if yield == nil {
		return errors.New("sqlite: model invocation reader is required")
	}
	rows, err := conn(ctx, m.db).QueryContext(ctx, `
		SELECT session_id, run_id, segment_id, call_id, started_at
		  FROM model_invocations
		 WHERE state = ?
		 ORDER BY session_id, run_id, segment_id, call_id
	`, modelInvocationStarted.databaseValue())
	if err != nil {
		return fmt.Errorf("sqlite: list started model invocations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sessionID, runID, segmentID, callID string
		var startedAt int64
		if err := rows.Scan(&sessionID, &runID, &segmentID, &callID, &startedAt); err != nil {
			return fmt.Errorf("sqlite: scan started model invocation: %w", err)
		}
		if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
			return fmt.Errorf("sqlite: restore started model invocation: %w", err)
		}
		if err := yield(sessionID, runID, segmentID, callID, time.Unix(0, startedAt).UTC()); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterate started model invocations: %w", err)
	}
	return nil
}

func (m *ModelInvocationStore) StartModelInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt time.Time,
) error {
	if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
		return err
	}
	if startedAt.IsZero() {
		return errors.New("sqlite: model invocation start time is required")
	}
	result, err := conn(ctx, m.db).ExecContext(ctx,
		`INSERT INTO model_invocations(
		   call_id, session_id, run_id, segment_id, state, started_at, finished_at)
		 SELECT ?, session_id, run_id, ?, ?, ?, 0
		   FROM runs
		  WHERE run_id = ? AND session_id = ? AND state != ?`,
		callID,
		segmentID,
		modelInvocationStarted.databaseValue(),
		startedAt.UTC().UnixNano(),
		runID,
		sessionID,
		runStateTerminal.databaseValue(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: start model invocation %q: %w", callID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect model invocation %q start: %w", callID, err)
	}
	if changed != 1 {
		return fmt.Errorf("sqlite: model invocation %q has no owning active Run", callID)
	}
	return nil
}

func (m *ModelInvocationStore) CompleteModelInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
	firstOutputLatencyMillis *int64,
	usage *accounting.TokenUsage,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationCompleted.databaseValue(), firstOutputLatencyMillis, usage,
	)
}

func (m *ModelInvocationStore) FailModelInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
	firstOutputLatencyMillis *int64,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationFailed.databaseValue(), firstOutputLatencyMillis, nil,
	)
}

func (m *ModelInvocationStore) MarkModelInvocationUnknown(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationUnknown.databaseValue(), nil, nil,
	)
}

func (m *ModelInvocationStore) finish(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
	state string,
	firstOutputLatencyMillis *int64,
	usage *accounting.TokenUsage,
) error {
	if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
		return err
	}
	if startedAt.IsZero() || finishedAt.IsZero() {
		return errors.New("sqlite: terminal model invocation requires start and finish times")
	}
	if finishedAt.Before(startedAt) {
		return errors.New("sqlite: model invocation finish time precedes start time")
	}
	if firstOutputLatencyMillis != nil && (*firstOutputLatencyMillis < 0 || (state != modelInvocationCompleted.databaseValue() && state != modelInvocationFailed.databaseValue())) {
		return errors.New("sqlite: invalid first output latency measurement")
	}
	encodedUsage, err := encodeModelInvocationUsage(usage)
	if err != nil {
		return err
	}
	result, err := conn(ctx, m.db).ExecContext(ctx,
		`UPDATE model_invocations
		    SET state = ?, finished_at = ?, first_output_latency_millis = ?, usage = ?
		  WHERE call_id = ? AND session_id = ? AND run_id = ? AND segment_id = ?
		    AND state = ? AND started_at = ?`,
		state,
		finishedAt.UTC().UnixNano(),
		firstOutputLatencyMillis,
		encodedUsage,
		callID,
		sessionID,
		runID,
		segmentID,
		modelInvocationStarted.databaseValue(),
		startedAt.UTC().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: finish model invocation %q as %s: %w", callID, state, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect model invocation %q transition: %w", callID, err)
	}
	if changed != 1 {
		return fmt.Errorf(
			"sqlite: model invocation %q no longer owns its started transition",
			callID,
		)
	}
	return nil
}

func validateModelInvocationIdentity(sessionID, runID, segmentID, callID string) error {
	if err := validateRunCoordinates("model invocation", sessionID, runID, segmentID); err != nil {
		return err
	}
	if err := runtimeidentity.ValidateEffect(callID); err != nil {
		return fmt.Errorf("sqlite: model invocation: %w", err)
	}
	return nil
}

// ModelInvocationRecord is a stored attempt, without semantic response content.
type ModelInvocationRecord struct {
	FirstOutputLatencyMillis *int64
	Usage                    *accounting.TokenUsage
	CallID                   string
	SegmentID                string
	State                    string
	StartedAt                time.Time
	FinishedAt               time.Time
}

// PageModelInvocations seeks newest-first within one Run. State updates do not
// move an attempt between pages because its start identity is immutable.
func (m *ModelInvocationStore) PageModelInvocations(ctx context.Context, runID string, beforeStartedAt int64, beforeCallID string, limit int) ([]ModelInvocationRecord, error) {
	query := `SELECT call_id, segment_id, state, started_at, finished_at, usage, first_output_latency_millis FROM model_invocations WHERE run_id = ?`
	args := []any{runID}
	if beforeCallID != "" {
		query += ` AND (started_at, call_id) < (?, ?)`
		args = append(args, beforeStartedAt, beforeCallID)
	}
	query += ` ORDER BY started_at DESC, call_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := conn(ctx, m.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: page model invocations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var records []ModelInvocationRecord
	for rows.Next() {
		var record ModelInvocationRecord
		var startedAt, finishedAt int64
		var usage sql.NullString
		var latency sql.NullInt64
		if err := rows.Scan(&record.CallID, &record.SegmentID, &record.State, &startedAt, &finishedAt, &usage, &latency); err != nil {
			return nil, fmt.Errorf("sqlite: scan model invocation: %w", err)
		}
		if latency.Valid {
			record.FirstOutputLatencyMillis = new(latency.Int64)
		}
		if usage.Valid {
			record.Usage, err = decodeModelInvocationUsage(usage.String)
			if err != nil {
				return nil, err
			}
		}
		record.StartedAt = time.Unix(0, startedAt).UTC()
		if finishedAt != 0 {
			record.FinishedAt = time.Unix(0, finishedAt).UTC()
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate model invocations: %w", err)
	}
	return records, nil
}
