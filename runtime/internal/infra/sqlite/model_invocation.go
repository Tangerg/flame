package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/instant"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
	"github.com/Tangerg/scope/core/chat"
)

type modelInvocationState string

const (
	modelInvocationStarted   modelInvocationState = "started"
	modelInvocationCompleted modelInvocationState = "completed"
	modelInvocationFailed    modelInvocationState = "failed"
	modelInvocationUnknown   modelInvocationState = "unknown"
)

func (m modelInvocationState) databaseValue() string { return string(m) }

// ModelInvocationStore records logical Interaction observations and distinct
// physical chat endpoints. Semantic continuation and aggregate accounting remain
// with history_items and runs; these observations never authorize provider replay.
type ModelInvocationStore struct{ db *sql.DB }

func NewModelInvocationStore(db *sql.DB) *ModelInvocationStore {
	return &ModelInvocationStore{db: db}
}

// ListStartedModelInvocations yields every provider attempt still owned by a
// pre-crash process. The callback keeps recovery application types out of the
// SQLite package while preserving an exact boot snapshot for its write-set.
func (m *ModelInvocationStore) ListStartedModelInvocations(
	ctx context.Context,
	yield func(sessionID, runID, segmentID, callID string, startedAt time.Time, purpose rundomain.ModelCallPurpose) error,
) error {
	if yield == nil {
		return errors.New("sqlite: model invocation reader is required")
	}
	rows, err := sqlitebase.Connection(ctx, m.db).QueryContext(ctx, `
		SELECT session_id, run_id, segment_id, call_id, started_at, logical_purpose
		  FROM model_invocations
		 WHERE state = ? AND endpoint_json IS NULL
		 ORDER BY session_id, run_id, segment_id, call_id
	`, modelInvocationStarted.databaseValue())
	if err != nil {
		return fmt.Errorf("sqlite: list started model invocations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sessionID, runID, segmentID, callID, rawPurpose string
		var startedAt int64
		if err := rows.Scan(&sessionID, &runID, &segmentID, &callID, &startedAt, &rawPurpose); err != nil {
			return fmt.Errorf("sqlite: scan started model invocation: %w", err)
		}
		if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
			return fmt.Errorf("sqlite: restore started model invocation: %w", err)
		}
		purpose, err := rundomain.ParseModelCallPurpose(rawPurpose)
		if err != nil {
			return err
		}
		if err := yield(sessionID, runID, segmentID, callID, time.Unix(0, startedAt).UTC(), purpose); err != nil {
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
	purpose rundomain.ModelCallPurpose,
) error {
	if err := purpose.Validate(); err != nil {
		return err
	}
	if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
		return err
	}
	if startedAt.IsZero() {
		return errors.New("sqlite: model invocation start time is required")
	}
	if err := instant.Validate(startedAt); err != nil {
		return fmt.Errorf("sqlite: model invocation start time: %w", err)
	}
	result, err := sqlitebase.Connection(ctx, m.db).ExecContext(ctx,
		`INSERT INTO model_invocations(
		   call_id, session_id, run_id, segment_id, state, started_at, finished_at, logical_purpose)
		 SELECT ?, session_id, run_id, ?, ?, ?, NULL, ?
		   FROM runs
		  WHERE run_id = ? AND session_id = ? AND state != ?`,
		callID,
		segmentID,
		modelInvocationStarted.databaseValue(),
		startedAt.UTC().UnixNano(),
		purpose.String(),
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
	usage *chat.Usage,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationCompleted.databaseValue(), firstOutputLatencyMillis, usage, nil,
	)
}

func (m *ModelInvocationStore) FailModelInvocation(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
	firstOutputLatencyMillis *int64,
	usage *chat.Usage, response *chat.Response,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationFailed.databaseValue(), firstOutputLatencyMillis, usage, response,
	)
}

func (m *ModelInvocationStore) MarkModelInvocationUnknown(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
) error {
	return m.finish(
		ctx, sessionID, runID, segmentID, callID,
		startedAt, finishedAt, modelInvocationUnknown.databaseValue(), nil, nil, nil,
	)
}

func (m *ModelInvocationStore) finish(
	ctx context.Context,
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
	state string,
	firstOutputLatencyMillis *int64,
	usage *chat.Usage,
	response *chat.Response,
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
	if err := instant.Validate(startedAt, finishedAt); err != nil {
		return fmt.Errorf("sqlite: model invocation times: %w", err)
	}
	if firstOutputLatencyMillis != nil && (*firstOutputLatencyMillis < 0 || (state != modelInvocationCompleted.databaseValue() && state != modelInvocationFailed.databaseValue())) {
		return errors.New("sqlite: invalid first output latency measurement")
	}
	encodedUsage, err := encodeModelInvocationUsage(usage)
	if err != nil {
		return err
	}
	var encodedResponse *string
	if response != nil {
		if state != modelInvocationFailed.databaseValue() {
			return errors.New("sqlite: known response requires failed outer invocation")
		}
		if err := response.Validate(); err != nil {
			return err
		}
		data, err := encodeStoredJSON(response)
		if err != nil {
			return err
		}
		encodedResponse = new(string(data))
		var reported *chat.Usage
		if response.Metadata != nil {
			reported = response.Metadata.Usage
		}
		if !reflect.DeepEqual(usage, reported) {
			return errors.New("sqlite: usage differs from known response")
		}
	} else if state == modelInvocationFailed.databaseValue() && usage != nil {
		return errors.New("sqlite: failed model usage requires a known response")
	}
	result, err := sqlitebase.Connection(ctx, m.db).ExecContext(ctx,
		`UPDATE model_invocations
		    SET state = ?, finished_at = ?, first_output_latency_millis = ?, usage = ?, known_response = ?
		  WHERE call_id = ? AND session_id = ? AND run_id = ? AND segment_id = ?
		    AND state = ? AND started_at = ?`,
		state,
		finishedAt.UTC().UnixNano(),
		firstOutputLatencyMillis,
		encodedUsage,
		encodedResponse,
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

// ModelInvocationRecord retains an attempt and any known response evidence;
// failed response evidence never authorizes conversation continuation.
type ModelInvocationRecord struct {
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
func (m *ModelInvocationStore) PageModelInvocations(ctx context.Context, runID string, beforeStartedAt int64, beforeCallID string, limit int) ([]ModelInvocationRecord, error) {
	query := `SELECT call_id, segment_id, state, started_at, finished_at, usage, first_output_latency_millis, known_response FROM model_invocations WHERE run_id = ? AND endpoint_json IS NULL`
	args := []any{runID}
	if beforeCallID != "" {
		query += ` AND (started_at, call_id) < (?, ?)`
		args = append(args, beforeStartedAt, beforeCallID)
	}
	query += ` ORDER BY started_at DESC, call_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := sqlitebase.Connection(ctx, m.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: page model invocations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var records []ModelInvocationRecord
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
}

func scanModelInvocation(row scanRow) (ModelInvocationRecord, error) {
	var record ModelInvocationRecord
	var startedAt int64
	var finishedAt sql.NullInt64
	var usage sql.NullString
	var response sql.NullString
	var latency sql.NullInt64
	if err := row.Scan(&record.CallID, &record.SegmentID, &record.State, &startedAt, &finishedAt, &usage, &latency, &response); err != nil {
		return ModelInvocationRecord{}, fmt.Errorf("sqlite: scan model invocation: %w", err)
	}
	if response.Valid {
		if record.State != modelInvocationFailed.databaseValue() {
			return ModelInvocationRecord{}, errors.New("sqlite: response evidence has invalid model state")
		}
		if err := decodeStoredJSON([]byte(response.String), &record.KnownResponse); err != nil {
			return ModelInvocationRecord{}, err
		}
		if record.KnownResponse == nil {
			return ModelInvocationRecord{}, errors.New("sqlite: null model response evidence")
		}
		if err := record.KnownResponse.Validate(); err != nil {
			return ModelInvocationRecord{}, err
		}
	}
	if latency.Valid {
		record.FirstOutputLatencyMillis = new(latency.Int64)
	}
	if usage.Valid {
		var err error
		record.Usage, err = decodeModelInvocationUsage(usage.String)
		if err != nil {
			return ModelInvocationRecord{}, err
		}
	}
	if record.KnownResponse != nil {
		var reported *chat.Usage
		if record.KnownResponse.Metadata != nil {
			reported = record.KnownResponse.Metadata.Usage
		}
		if !reflect.DeepEqual(record.Usage, reported) {
			return ModelInvocationRecord{}, errors.New("sqlite: retained usage differs from known response")
		}
	}
	record.StartedAt = time.Unix(0, startedAt).UTC()
	if finishedAt.Valid {
		record.FinishedAt = time.Unix(0, finishedAt.Int64).UTC()
	}
	return record, nil
}
