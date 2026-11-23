package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

type ModelInvocationStore struct{ db *database }

func newModelInvocationStore(db *database) *ModelInvocationStore {
	return &ModelInvocationStore{db: db}
}

func (m *ModelInvocationStore) ListOpenModelInvocations(ctx context.Context) ([]runs.OpenModelInvocation, error) {
	return scopedValue(ctx, m.db.read, func(ctx context.Context) ([]runs.OpenModelInvocation, error) {
		rows, err := m.db.connection(ctx).QueryContext(ctx, `
		SELECT session_id, run_id, segment_id, call_id, started_at, logical_purpose
		  FROM model_invocations
		 WHERE state = ? AND endpoint_json IS NULL
		 ORDER BY session_id, run_id, segment_id, call_id
	`, string(modelref.InvocationStarted))
		if err != nil {
			return nil, fmt.Errorf("sqlite: list started model invocations: %w", err)
		}
		defer rows.Close()
		var open []runs.OpenModelInvocation
		for rows.Next() {
			var invocation runs.OpenModelInvocation
			var rawPurpose string
			var startedAt int64
			if err := rows.Scan(&invocation.SessionID, &invocation.RunID, &invocation.SegmentID, &invocation.CallID, &startedAt, &rawPurpose); err != nil {
				return nil, fmt.Errorf("sqlite: scan started model invocation: %w", err)
			}
			if invocation.Purpose, err = rundomain.ParseModelCallPurpose(rawPurpose); err != nil {
				return nil, err
			}
			invocation.StartedAt = time.Unix(0, startedAt).UTC()
			open = append(open, invocation)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: iterate started model invocations: %w", err)
		}
		for index, invocation := range open {
			source, found, err := m.EndpointForCaller(ctx, invocation.CallID)
			if err != nil {
				return nil, err
			}
			if found {
				open[index].Endpoint = &source
			}
			if err := open[index].Validate(); err != nil {
				return nil, fmt.Errorf("sqlite: restore started model invocation: %w", err)
			}
		}
		return open, nil
	})
}

func (m *ModelInvocationStore) RecordModelInvocation(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ModelInvocationCommit) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	if err := validateRunCoordinates("model invocation", sessionID, runID, invocation.SegmentID); err != nil {
		return err
	}
	if invocation.State == modelref.InvocationStarted {
		return m.start(ctx, sessionID, runID, invocation)
	}
	return m.finish(ctx, sessionID, runID, invocation)
}

func (m *ModelInvocationStore) start(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ModelInvocationCommit) error {
	return m.db.write(ctx, func(ctx context.Context) error {
		if err := instant.Validate(invocation.StartedAt); err != nil {
			return fmt.Errorf("sqlite: model invocation start time: %w", err)
		}
		result, err := m.db.connection(ctx).ExecContext(ctx, `INSERT INTO model_invocations(
		   call_id, session_id, run_id, segment_id, state, started_at, finished_at, logical_purpose)
		 SELECT ?, session_id, run_id, ?, ?, ?, NULL, ?
		   FROM runs
		  WHERE run_id = ? AND session_id = ? AND state = ? AND active_segment_id = ?`, invocation.CallID, invocation.SegmentID, string(modelref.InvocationStarted), invocation.StartedAt.UTC().UnixNano(), invocation.Purpose.String(), runID, sessionID, string(rundomain.Running), invocation.SegmentID)
		if err != nil {
			return fmt.Errorf("sqlite: start model invocation %q: %w", invocation.CallID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect model invocation %q start: %w", invocation.CallID, err)
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: model invocation %q has no owning active Run", invocation.CallID)
		}
		return nil
	})
}

func (m *ModelInvocationStore) finish(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ModelInvocationCommit) error {
	return m.db.write(ctx, func(ctx context.Context) error {
		if err := instant.Validate(invocation.StartedAt, invocation.FinishedAt); err != nil {
			return fmt.Errorf("sqlite: model invocation times: %w", err)
		}
		encodedUsage, err := encodeModelInvocationUsage(invocation.Usage)
		if err != nil {
			return err
		}
		var encodedResponse *string
		if invocation.KnownResponse != nil {
			data, err := encodeStoredJSON(invocation.KnownResponse)
			if err != nil {
				return err
			}
			encodedResponse = new(string(data))
		}
		result, err := m.db.connection(ctx).ExecContext(ctx, `UPDATE model_invocations
		    SET state = ?, finished_at = ?, first_output_latency_millis = ?, usage = ?, known_response = ?
		  WHERE call_id = ? AND session_id = ? AND run_id = ? AND segment_id = ?
		    AND state = ? AND started_at = ?`, string(invocation.State), invocation.FinishedAt.UTC().UnixNano(), invocation.FirstOutputLatencyMillis, encodedUsage, encodedResponse, invocation.CallID, sessionID, runID, invocation.SegmentID, string(modelref.InvocationStarted), invocation.StartedAt.UTC().UnixNano())
		if err != nil {
			return fmt.Errorf("sqlite: finish model invocation %q as %s: %w", invocation.CallID, invocation.State, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect model invocation %q transition: %w", invocation.CallID, err)
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: model invocation %q no longer owns its started transition", invocation.CallID)
		}
		return nil
	})
}

const modelInvocationColumns = `call_id, segment_id, logical_purpose, state, started_at, finished_at, usage, first_output_latency_millis, known_response`

func (m *ModelInvocationStore) PageModelInvocations(ctx context.Context, runID identity.RunID, beforeStartedAt int64, beforeCallID string, limit int) ([]runs.ModelInvocationCommit, error) {
	return scopedValue(ctx, m.db.read, func(ctx context.Context) ([]runs.ModelInvocationCommit, error) {
		query := `SELECT ` + modelInvocationColumns + ` FROM model_invocations WHERE run_id = ? AND endpoint_json IS NULL`
		args := []any{runID}
		if beforeCallID != "" {
			query += ` AND (started_at, call_id) < (?, ?)`
			args = append(args, beforeStartedAt, beforeCallID)
		}
		query += ` ORDER BY started_at DESC, call_id DESC LIMIT ?`
		args = append(args, limit)
		rows, err := m.db.connection(ctx).QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("sqlite: page model invocations: %w", err)
		}
		defer rows.Close()
		var records []runs.ModelInvocationCommit
		for rows.Next() {
			record, err := scanModelInvocation(rows)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: iterate model invocations: %w", err)
		}
		return records, nil
	})
}

func scanModelInvocation(row scanRow) (runs.ModelInvocationCommit, error) {
	var record runs.ModelInvocationCommit
	var purpose, state string
	var startedAt int64
	var finishedAt sql.NullInt64
	var usage sql.NullString
	var response sql.NullString
	var latency sql.NullInt64
	if err := row.Scan(&record.CallID, &record.SegmentID, &purpose, &state, &startedAt, &finishedAt, &usage, &latency, &response); err != nil {
		return runs.ModelInvocationCommit{}, fmt.Errorf("sqlite: scan model invocation: %w", err)
	}
	var err error
	if record.Purpose, err = rundomain.ParseModelCallPurpose(purpose); err != nil {
		return runs.ModelInvocationCommit{}, err
	}
	record.State = modelref.InvocationState(state)
	if response.Valid {
		if err := decodeStoredJSON([]byte(response.String), &record.KnownResponse); err != nil {
			return runs.ModelInvocationCommit{}, err
		}
		if record.KnownResponse == nil {
			return runs.ModelInvocationCommit{}, errors.New("sqlite: null model response evidence")
		}
	}
	if latency.Valid {
		record.FirstOutputLatencyMillis = new(latency.Int64)
	}
	if usage.Valid {
		if record.Usage, err = decodeModelInvocationUsage(usage.String); err != nil {
			return runs.ModelInvocationCommit{}, err
		}
	}
	record.StartedAt = time.Unix(0, startedAt).UTC()
	if finishedAt.Valid {
		record.FinishedAt = time.Unix(0, finishedAt.Int64).UTC()
	}
	if err := record.Validate(); err != nil {
		return runs.ModelInvocationCommit{}, fmt.Errorf("sqlite: restore model invocation: %w", err)
	}
	return record, nil
}
