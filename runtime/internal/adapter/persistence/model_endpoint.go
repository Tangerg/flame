package persistence

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	"reflect"
	"time"
)

const endpointObservationColumns = `call_id,started_at,endpoint_json,state,finished_at,known_response,endpoint_failure,COALESCE(session_id,''),COALESCE(run_id,''),COALESCE(segment_id,''),endpoint_cost_usd,usage,COALESCE(endpoint_kind,'')`

type storedEndpointInvocation struct {
	ID        string                `json:"id"`
	Kind      modelref.EndpointKind `json:"kind"`
	Caller    portable.Invocation   `json:"caller"`
	Provider  string                `json:"provider"`
	Model     string                `json:"model"`
	SessionID string                `json:"sessionId,omitzero"`
	Request   *chat.Request         `json:"request,omitzero"`
	Embedding *embedding.Request    `json:"embedding,omitzero"`
	StartedAt time.Time             `json:"startedAt"`
}

func encodeEndpointInvocation(value modelref.EndpointInvocation) ([]byte, error) {
	data, err := encodeStoredJSON(storedEndpointInvocation{ID: value.ID, Kind: value.Kind, Caller: value.Caller, Provider: value.Provider, Model: value.Model, SessionID: value.SessionID, Request: value.Request, Embedding: value.Embedding, StartedAt: value.StartedAt})
	if err != nil {
		return nil, err
	}
	input, err := jsonvalue.Parse(data)
	if err != nil {
		return nil, err
	}
	return []byte(input.String()), nil
}

func encodeEndpointOutcome(invocation modelref.EndpointInvocation, outcome modelref.EndpointOutcome) (*string, error) {
	var response any
	if invocation.Kind == modelref.EndpointChat {
		if outcome.Response == nil {
			return nil, nil
		}
		response = outcome.Response
	} else {
		if outcome.EmbeddingResponse == nil {
			return nil, nil
		}
		response = outcome.EmbeddingResponse
	}
	data, err := encodeStoredJSON(response)
	if err != nil {
		return nil, err
	}
	value, err := jsonvalue.Parse(data)
	if err != nil {
		return nil, err
	}
	return new(value.String()), nil
}

// Caller IDs and wire parents describe ancestry, not execution authority. The
// explicit logical owner supplies a frozen association; absence means no Run.
func (m *ModelInvocationStore) StartEndpointInvocation(ctx context.Context, invocation modelref.EndpointInvocation, ownerCallID string) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	if err := instant.Validate(invocation.StartedAt); err != nil {
		return err
	}
	encoded, err := encodeEndpointInvocation(invocation)
	if err != nil {
		return err
	}
	var session any
	if invocation.SessionID != "" {
		session = invocation.SessionID
	}

	scope := `SELECT ? session_id, NULL run_id, NULL segment_id`
	args := []any{session}
	if ownerCallID != "" {
		if err := runs.ValidateInvocation(ownerCallID); err != nil {
			return err
		}
		if invocation.SessionID == "" {
			return errors.New("sqlite: endpoint execution owner requires its captured Session")
		}
		scope = `SELECT owner.session_id, owner.run_id, owner.segment_id FROM (
			SELECT session_id,run_id,segment_id,started_at FROM model_invocations
			WHERE call_id=? AND endpoint_json IS NULL AND state=?
			UNION ALL SELECT session_id,run_id,segment_id,started_at FROM tool_invocations
			WHERE call_id=? AND state=?
		) owner JOIN runs ON runs.run_id=owner.run_id AND runs.session_id=owner.session_id
		WHERE runs.state=? AND runs.active_segment_id=owner.segment_id AND owner.started_at<=?`
		args = []any{ownerCallID, runs.ModelInvocationStarted.String(), ownerCallID, toolInvocationStarted.databaseValue(), runStateRunning.databaseValue(), invocation.StartedAt.UTC().UnixNano()}
	}
	query := `INSERT INTO model_invocations(call_id,session_id,run_id,segment_id,state,started_at,endpoint_json,endpoint_kind)
 SELECT ?,scope.session_id,scope.run_id,scope.segment_id,?,?,?,? FROM (` + scope + `) scope
 WHERE (scope.session_id IS NULL OR EXISTS(SELECT 1 FROM sessions WHERE id=scope.session_id))
 AND (? IS NULL OR scope.session_id=?)
 AND NOT EXISTS(SELECT 1 FROM model_invocations WHERE endpoint_json IS NULL AND call_id=? AND call_id!=?)
 AND NOT EXISTS(SELECT 1 FROM tool_invocations WHERE call_id=?)`
	parameters := []any{invocation.ID, modelref.EndpointStarted, invocation.StartedAt.UTC().UnixNano(), string(encoded), string(invocation.Kind)}
	parameters = append(parameters, args...)
	parameters = append(parameters, session, session, invocation.Caller.ID, ownerCallID, invocation.Caller.ID)
	result, err := sqlite.Connection(ctx, m.db).ExecContext(ctx, query, parameters...)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("sqlite: endpoint has no matching active execution owner or Session")
	}
	return nil
}

func (m *ModelInvocationStore) SettleEndpointInvocation(ctx context.Context, invocation modelref.EndpointInvocation, outcome modelref.EndpointOutcome) error {
	state, err := outcome.State(invocation)
	if err != nil {
		return err
	}
	response, err := encodeEndpointOutcome(invocation, outcome)
	if err != nil {
		return err
	}
	usage := outcome.ReportedUsage()
	encodedUsage, err := encodeModelInvocationUsage(usage)
	if err != nil {
		return err
	}
	encoded, err := encodeEndpointInvocation(invocation)
	if err != nil {
		return err
	}
	result, err := sqlite.Connection(ctx, m.db).ExecContext(ctx, `UPDATE model_invocations SET state=?,finished_at=?,known_response=?,usage=?,endpoint_failure=?,endpoint_cost_usd=?
		WHERE call_id=? AND state=? AND started_at=? AND endpoint_json=?`, state, outcome.FinishedAt.UTC().UnixNano(), response, encodedUsage, outcome.Failure, outcome.CostUSD,
		invocation.ID, modelref.EndpointStarted, invocation.StartedAt.UTC().UnixNano(), string(encoded))
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("sqlite: endpoint observation is no longer open")
	}
	return nil
}

func (m *ModelInvocationStore) PageEndpointInvocations(ctx context.Context, parent string, before int64, beforeID string, limit int) ([]modelref.EndpointObservation, error) {
	query := `SELECT ` + endpointObservationColumns + ` FROM model_invocations WHERE endpoint_json IS NOT NULL AND (json_extract(endpoint_json,'$.caller.id')=? OR json_extract(endpoint_json,'$.caller.parent')=?)`
	args := []any{parent, parent}
	if beforeID != "" {
		query += ` AND (started_at,call_id)<(?,?)`
		args = append(args, before, beforeID)
	}
	query += ` ORDER BY started_at DESC,call_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := sqlite.Connection(ctx, m.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []modelref.EndpointObservation{}
	for rows.Next() {
		value, err := scanEndpointObservation(rows, parent)
		if err != nil {
			return nil, err
		}

		values = append(values, value)
	}
	return values, rows.Err()
}

func scanEndpointObservation(row scanRow, parent string) (modelref.EndpointObservation, error) {
	var err error
	var value modelref.EndpointRecord
	var encoded string
	var id string
	var startedAt int64
	var finishedAt *int64
	var response *string
	var encodedUsage *string
	var kind string
	if err := row.Scan(&id, &startedAt, &encoded, &value.State, &finishedAt, &response, &value.Failure, &value.SessionID, &value.RunID, &value.SegmentID, &value.CostUSD, &encodedUsage, &kind); err != nil {
		return modelref.EndpointObservation{}, err
	}
	value.InvocationJSON, err = jsonvalue.Parse([]byte(encoded))
	if err != nil {
		return modelref.EndpointObservation{}, err
	}
	var admission storedEndpointInvocation
	if err := decodeStoredJSON([]byte(encoded), &admission); err != nil {
		return modelref.EndpointObservation{}, err
	}
	if modelref.EndpointKind(kind) != admission.Kind {
		return modelref.EndpointObservation{}, errors.New("sqlite: endpoint kind differs from its admission")
	}
	value.Invocation = modelref.EndpointInvocation{ID: admission.ID, Kind: admission.Kind, Caller: admission.Caller, Provider: admission.Provider, Model: admission.Model, SessionID: admission.SessionID, Request: admission.Request, Embedding: admission.Embedding, StartedAt: admission.StartedAt}
	var fields map[string]jsontext.Value
	if err := decodeStoredJSON([]byte(encoded), &fields); err != nil {
		return modelref.EndpointObservation{}, err
	}
	requestField := "request"
	if admission.Kind == modelref.EndpointEmbedding {
		requestField = "embedding"
	}
	value.RequestJSON, err = jsonvalue.Parse(fields[requestField])
	if err != nil {
		return modelref.EndpointObservation{}, err
	}
	if value.Invocation.ID != id || value.Invocation.StartedAt.UTC().UnixNano() != startedAt || (parent != "" && value.Invocation.Caller.ID != parent && value.Invocation.Caller.Parent != parent) {
		return modelref.EndpointObservation{}, errors.New("sqlite: endpoint observation identity differs from its row")
	}
	if response != nil {
		value.ResponseJSON, err = jsonvalue.Parse([]byte(*response))
		if err != nil {
			return modelref.EndpointObservation{}, err
		}
	}
	if encodedUsage != nil {
		value.Usage, err = decodeModelInvocationUsage(*encodedUsage)
		if err != nil {
			return modelref.EndpointObservation{}, err
		}
	}
	if finishedAt != nil {
		value.FinishedAt = time.Unix(0, *finishedAt).UTC()
	}
	return modelref.RestoreEndpointObservation(value)
}

func (m *ModelInvocationStore) ReconcileEndpointInvocations(ctx context.Context, settledAt time.Time) error {
	if err := instant.Validate(settledAt); err != nil {
		return err
	}
	_, err := sqlite.Connection(ctx, m.db).ExecContext(ctx, `UPDATE model_invocations SET state=?,finished_at=MAX(started_at,?),endpoint_failure='process stopped before endpoint completion was recorded' WHERE endpoint_json IS NOT NULL AND state=?`, modelref.EndpointUnknown, settledAt.UTC().UnixNano(), modelref.EndpointStarted)
	return err
}

func (m *ModelInvocationStore) EndpointForCaller(ctx context.Context, caller string) (modelref.EndpointObservation, bool, error) {
	row := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT `+endpointObservationColumns+` FROM model_invocations WHERE endpoint_json IS NOT NULL AND json_extract(endpoint_json,'$.caller.id')=?`, caller)
	value, err := scanEndpointObservation(row, caller)
	if errors.Is(err, sql.ErrNoRows) {
		return modelref.EndpointObservation{}, false, nil
	}
	return value, err == nil, err
}

// ListSessionEndpoints materializes evidence after the export caller checks
// source limits in the same transaction. Ordinary inspection uses bounded pages.
func (m *ModelInvocationStore) ListSessionEndpoints(ctx context.Context, sessionID string) ([]modelref.EndpointObservation, error) {
	if err := identity.ValidateSession(sessionID); err != nil {
		return nil, err
	}
	rows, err := sqlite.Connection(ctx, m.db).QueryContext(ctx, `SELECT `+endpointObservationColumns+` FROM model_invocations WHERE endpoint_json IS NOT NULL AND session_id=? ORDER BY started_at,call_id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []modelref.EndpointObservation{}
	for rows.Next() {
		value, err := scanEndpointObservation(rows, "")
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (m *ModelInvocationStore) trajectoryEndpoint(ctx context.Context, sessionID, id string) (modelref.EndpointObservation, error) {
	row := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT `+endpointObservationColumns+` FROM model_invocations WHERE endpoint_json IS NOT NULL AND session_id=? AND call_id=?`, sessionID, id)
	return scanEndpointObservation(row, "")
}

// RecoverModelInvocation projects known physical evidence into a failed logical
// attempt, or closes an unobserved attempt as unknown. The source CAS joins the
// existing Run recovery transaction and never permits replay or continuation.
func (m *ModelInvocationStore) RecoverModelInvocation(ctx context.Context, sessionID, runID, segmentID, callID string, startedAt, finishedAt time.Time, source *modelref.EndpointObservation, purpose rundomain.ModelCallPurpose) error {
	if err := purpose.Validate(); err != nil {
		return err
	}
	var actualPurpose string
	if err := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT logical_purpose FROM model_invocations WHERE call_id=? AND endpoint_json IS NULL`, callID).Scan(&actualPurpose); err != nil {
		return err
	}
	if actualPurpose != purpose.String() {
		return errors.New("sqlite: recovered model purpose changed")
	}
	if source != nil {
		record := source.Snapshot()
		if record.RunID != runID || record.SessionID != sessionID || record.SegmentID != segmentID || record.Invocation.Caller.ID != callID || record.Invocation.StartedAt.Before(startedAt) || record.FinishedAt.After(finishedAt) {
			return errors.New("sqlite: recovered endpoint differs from original call")
		}
		if err := m.validateEndpointProjection(ctx, *source); err != nil {
			return err
		}
	}
	if source == nil || !source.HasResponse() {
		return m.MarkModelInvocationUnknown(ctx, sessionID, runID, segmentID, callID, startedAt, finishedAt)
	}
	if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
		return err
	}
	if err := instant.Validate(startedAt, finishedAt); err != nil {
		return err
	}
	record := source.Snapshot()
	if err := record.ResponseJSON.Validate(); err != nil {
		return err
	}
	if err := record.InvocationJSON.Validate(); err != nil {
		return err
	}
	usage, err := encodeModelInvocationUsage(record.Usage)
	if err != nil {
		return err
	}
	result, err := sqlite.Connection(ctx, m.db).ExecContext(ctx, `WITH evidence AS (
	 SELECT usage,known_response FROM model_invocations WHERE call_id=? AND endpoint_json=? AND session_id=? AND run_id=? AND segment_id=?
	 AND state=? AND finished_at=? AND known_response=? AND endpoint_cost_usd IS ? AND endpoint_failure=? AND usage IS ?)
	 UPDATE model_invocations SET state=?,finished_at=?,usage=(SELECT usage FROM evidence),known_response=(SELECT known_response FROM evidence)
	 WHERE call_id=? AND session_id=? AND run_id=? AND segment_id=? AND state=? AND started_at=? AND endpoint_json IS NULL AND EXISTS(SELECT 1 FROM evidence)`,
		record.Invocation.ID, record.InvocationJSON.String(), sessionID, runID, segmentID, record.State, record.FinishedAt.UTC().UnixNano(), record.ResponseJSON.String(), record.CostUSD, record.Failure, usage,
		runs.ModelInvocationFailed.String(), finishedAt.UTC().UnixNano(), callID, sessionID, runID, segmentID, runs.ModelInvocationStarted.String(), startedAt.UTC().UnixNano())
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("sqlite: recovered endpoint or logical call changed before settlement")
	}
	return nil
}

// VisitEndpointInvocations reads one coherent retained source snapshot with one
// body materialized at a time. It includes Session-only and unattached calls;
// aggregate consumers never fold the logical Run projections a second time.
func (m *ModelInvocationStore) VisitEndpointInvocations(ctx context.Context, sessionID string, since time.Time, yield func(modelref.EndpointObservation) error) error {
	if yield == nil {
		return errors.New("sqlite: endpoint usage consumer is required")
	}
	if sessionID != "" {
		if err := identity.ValidateSession(sessionID); err != nil {
			return err
		}
	}
	if !since.IsZero() {
		if err := instant.Validate(since); err != nil {
			return err
		}
	}
	return sqlite.RunReadSnapshot(ctx, m.db, func(ctx context.Context) error {
		if sessionID != "" {
			var exists bool
			if err := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=?)`, sessionID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return session.ErrNotFound
			}
		}
		ownership := `SELECT EXISTS(SELECT 1 FROM model_invocations source LEFT JOIN runs owner ON owner.run_id=source.run_id LEFT JOIN sessions scope ON scope.id=source.session_id WHERE source.endpoint_json IS NOT NULL AND ((source.run_id IS NOT NULL AND (owner.run_id IS NULL OR source.session_id IS NULL OR source.session_id!=owner.session_id)) OR (source.session_id IS NOT NULL AND scope.id IS NULL))`
		var ownershipArgs []any
		if sessionID != "" {
			ownership += ` AND (source.session_id=? OR owner.session_id=?)`
			ownershipArgs = append(ownershipArgs, sessionID, sessionID)
		}
		if !since.IsZero() {
			ownership += ` AND source.started_at>=?`
			ownershipArgs = append(ownershipArgs, since.UTC().UnixNano())
		}
		ownership += `)`
		var invalid bool
		if err := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, ownership, ownershipArgs...).Scan(&invalid); err != nil {
			return err
		}
		if invalid {
			return errors.New("sqlite: physical usage source has inconsistent ownership")
		}
		query := `SELECT ` + endpointObservationColumns + ` FROM model_invocations WHERE endpoint_json IS NOT NULL`
		var args []any
		if sessionID != "" {
			query += ` AND session_id=?`
			args = append(args, sessionID)
		}
		if !since.IsZero() {
			query += ` AND started_at>=?`
			args = append(args, since.UTC().UnixNano())
		}
		query += ` ORDER BY started_at,call_id`
		rows, err := sqlite.Connection(ctx, m.db).QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			value, err := scanEndpointObservation(rows, "")
			if err != nil {
				return err
			}
			if err := yield(value); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

// ListIndependentEndpoints returns retained descendant attempts not projected
// through a logical Model receipt. Active Run recovery owns its one accrual.
func (m *ModelInvocationStore) ListIndependentEndpoints(ctx context.Context) ([]modelref.EndpointObservation, error) {
	rows, err := sqlite.Connection(ctx, m.db).QueryContext(ctx, `SELECT `+endpointObservationColumns+` FROM model_invocations
 WHERE endpoint_json IS NOT NULL AND run_id IN (SELECT run_id FROM runs WHERE state!=?)
 AND NOT EXISTS(SELECT 1 FROM model_invocations logical WHERE logical.endpoint_json IS NULL AND logical.call_id=json_extract(model_invocations.endpoint_json,'$.caller.id'))
 ORDER BY session_id,run_id,call_id`, runStateTerminal.databaseValue())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []modelref.EndpointObservation{}
	for rows.Next() {
		value, err := scanEndpointObservation(rows, "")
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// ValidateIndependentEndpoint fences the recovery projection to the immutable
// physical source in the same transaction as its Run/Goal replacement.
func (m *ModelInvocationStore) validateEndpointProjection(ctx context.Context, expected modelref.EndpointObservation) error {
	record := expected.Snapshot()
	source, found, err := m.EndpointForCaller(ctx, record.Invocation.Caller.ID)
	if err != nil {
		return err
	}
	if !found || record.RunID == "" || !reflect.DeepEqual(source.Snapshot(), record) {
		return errors.New("sqlite: independent endpoint recovery source changed")
	}
	return nil
}

func (m *ModelInvocationStore) ValidateIndependentEndpoint(ctx context.Context, expected modelref.EndpointObservation) error {
	if err := m.validateEndpointProjection(ctx, expected); err != nil {
		return err
	}

	var logical int
	if err := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT count(*) FROM model_invocations WHERE call_id=? AND endpoint_json IS NULL`, expected.Snapshot().Invocation.Caller.ID).Scan(&logical); err != nil {
		return err
	}
	if logical != 0 {
		return errors.New("sqlite: independent endpoint has a logical accounting owner")
	}
	return nil
}
