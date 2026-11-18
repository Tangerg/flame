package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
)

// ModelInvocationStore records logical Interaction observations and distinct
// physical provider endpoints. Semantic continuation and aggregate accounting remain
// with history_items and runs; these observations never authorize provider replay.
type ModelInvocationStore struct{ db *database }

func newModelInvocationStore(db *database) *ModelInvocationStore {
	return &ModelInvocationStore{db: db}
}

// ListOpenModelInvocations returns every provider attempt still owned by a
// pre-crash process, with the physical endpoint it reached when one was observed.
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
		defer func() {
			_ = rows.Close()
		}()
		var open []runs.OpenModelInvocation
		for rows.Next() {
			var invocation runs.OpenModelInvocation
			var rawPurpose string
			var startedAt int64
			if err := rows.Scan(&invocation.SessionID, &invocation.RunID, &invocation.SegmentID, &invocation.CallID, &startedAt, &rawPurpose); err != nil {
				return nil, fmt.Errorf("sqlite: scan started model invocation: %w", err)
			}
			if err := validateModelInvocationIdentity(invocation.SessionID, invocation.RunID, invocation.SegmentID, invocation.CallID); err != nil {
				return nil, fmt.Errorf("sqlite: restore started model invocation: %w", err)
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
			if !found {
				continue
			}
			record := source.Snapshot()
			if record.SessionID != invocation.SessionID || record.RunID != invocation.RunID || record.Invocation.Caller.ID != invocation.CallID {
				return nil, errors.New("sqlite: endpoint differs from its logical call ownership")
			}
			open[index].Endpoint = &source
		}
		return open, nil
	})
}

// RecordModelInvocation applies one journal transition committed with its Run event.
func (m *ModelInvocationStore) RecordModelInvocation(ctx context.Context, sessionID, runID string, invocation runs.ModelInvocationCommit) error {
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

func (m *ModelInvocationStore) start(ctx context.Context, sessionID, runID string, invocation runs.ModelInvocationCommit) error {
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

func (m *ModelInvocationStore) finish(ctx context.Context, sessionID, runID string, invocation runs.ModelInvocationCommit) error {
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
func validateModelInvocationIdentity(sessionID, runID, segmentID, callID string) error {
	if err := validateRunCoordinates("model invocation", sessionID, runID, segmentID); err != nil {
		return err
	}
	if err := runs.ValidateInvocation(callID); err != nil {
		return fmt.Errorf("sqlite: model invocation: %w", err)
	}
	return nil
}

// modelInvocationRecord retains an attempt and any known response evidence;
// failed response evidence never authorizes conversation continuation.
type modelInvocationRecord struct {
	KnownResponse            *chat.Response
	FirstOutputLatencyMillis *int64
	Usage                    *chat.Usage
	CallID                   string
	SegmentID                string
	State                    string
	StartedAt                time.Time
	FinishedAt               time.Time
}

// PageModelInvocations seeks newest-first within one Run. State updates do not
// move an attempt between pages because its start identity is immutable.
func (m *ModelInvocationStore) PageModelInvocations(ctx context.Context, runID string, beforeStartedAt int64, beforeCallID string, limit int) ([]modelInvocationRecord, error) {
	return scopedValue(ctx, m.db.read, func(ctx context.Context) ([]modelInvocationRecord, error) {
		query := `SELECT call_id, segment_id, state, started_at, finished_at, usage, first_output_latency_millis, known_response FROM model_invocations WHERE run_id = ? AND endpoint_json IS NULL`
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
		defer func() {
			_ = rows.Close()
		}()
		var records []modelInvocationRecord
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
func scanModelInvocation(row scanRow) (modelInvocationRecord, error) {
	var record modelInvocationRecord
	var startedAt int64
	var finishedAt sql.NullInt64
	var usage sql.NullString
	var response sql.NullString
	var latency sql.NullInt64
	if err := row.Scan(&record.CallID, &record.SegmentID, &record.State, &startedAt, &finishedAt, &usage, &latency, &response); err != nil {
		return modelInvocationRecord{}, fmt.Errorf("sqlite: scan model invocation: %w", err)
	}
	if response.Valid {
		if record.State != string(modelref.InvocationFailed) {
			return modelInvocationRecord{}, errors.New("sqlite: response evidence has invalid model state")
		}
		if err := decodeStoredJSON([]byte(response.String), &record.KnownResponse); err != nil {
			return modelInvocationRecord{}, err
		}
		if record.KnownResponse == nil {
			return modelInvocationRecord{}, errors.New("sqlite: null model response evidence")
		}
		if err := record.KnownResponse.Validate(); err != nil {
			return modelInvocationRecord{}, err
		}
	}
	if latency.Valid {
		record.FirstOutputLatencyMillis = new(latency.Int64)
	}
	if usage.Valid {
		var err error
		record.Usage, err = decodeModelInvocationUsage(usage.String)
		if err != nil {
			return modelInvocationRecord{}, err
		}
	}
	if record.KnownResponse != nil {
		if !modelref.UsageMatchesResponse(record.Usage, record.KnownResponse) {
			return modelInvocationRecord{}, errors.New("sqlite: retained usage differs from known response")
		}
	}
	record.StartedAt = time.Unix(0, startedAt).UTC()
	if finishedAt.Valid {
		record.FinishedAt = time.Unix(0, finishedAt.Int64).UTC()
	}
	return record, nil
}
