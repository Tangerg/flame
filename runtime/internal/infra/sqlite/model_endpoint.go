package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"reflect"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/instant"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/scope/core/chat"
)

const endpointObservationColumns = `call_id,started_at,endpoint_json,state,finished_at,known_response,endpoint_failure,COALESCE(session_id,''),COALESCE(run_id,''),COALESCE(segment_id,''),endpoint_cost_usd,usage`

type storedEndpointInvocation struct {
	ID        string                `json:"id"`
	Caller    capability.Invocation `json:"caller"`
	Provider  string                `json:"provider"`
	Model     string                `json:"model"`
	SessionID string                `json:"sessionId,omitzero"`
	Request   *chat.Request         `json:"request"`
	StartedAt time.Time             `json:"startedAt"`
}

func encodeEndpointInvocation(value modelref.EndpointInvocation) ([]byte, error) {
	data, err := encodeStoredJSON(storedEndpointInvocation{ID: value.ID, Caller: value.Caller, Provider: value.Provider, Model: value.Model, SessionID: value.SessionID, Request: value.Request, StartedAt: value.StartedAt})
	if err != nil {
		return nil, err
	}
	input, err := jsoninput.Parse(data)
	if err != nil {
		return nil, err
	}
	return []byte(input.String()), nil
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
		if err := runtimeidentity.ValidateEffect(ownerCallID); err != nil {
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
		args = []any{ownerCallID, modelInvocationStarted.databaseValue(), ownerCallID, toolInvocationStarted.databaseValue(), runStateRunning.databaseValue(), invocation.StartedAt.UTC().UnixNano()}
	}
	query := `INSERT INTO model_invocations(call_id,session_id,run_id,segment_id,state,started_at,endpoint_json)
 SELECT ?,scope.session_id,scope.run_id,scope.segment_id,?,?,? FROM (` + scope + `) scope
 WHERE (scope.session_id IS NULL OR EXISTS(SELECT 1 FROM sessions WHERE id=scope.session_id))
 AND (? IS NULL OR scope.session_id=?)
 AND NOT EXISTS(SELECT 1 FROM model_invocations WHERE endpoint_json IS NULL AND call_id=? AND call_id!=?)
 AND NOT EXISTS(SELECT 1 FROM tool_invocations WHERE call_id=?)`
	parameters := []any{invocation.ID, modelInvocationStarted.databaseValue(), invocation.StartedAt.UTC().UnixNano(), string(encoded)}
	parameters = append(parameters, args...)
	parameters = append(parameters, session, session, invocation.Caller.ID, ownerCallID, invocation.Caller.ID)
	result, err := Connection(ctx, m.db).ExecContext(ctx, query, parameters...)
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
	if err := invocation.Validate(); err != nil {
		return err
	}
	if _, err := accounting.CostFromOptional(outcome.CostUSD); err != nil {
		return err
	}
	if outcome.CostUSD != nil && (outcome.Response == nil || outcome.Response.Metadata == nil || outcome.Response.Metadata.Usage == nil) {
		return errors.New("sqlite: endpoint price requires known reported usage")
	}
	if outcome.Response == nil && outcome.Failure == "" {
		return errors.New("sqlite: missing endpoint response requires uncertainty evidence")
	}
	if outcome.FinishedAt.IsZero() || outcome.FinishedAt.Before(invocation.StartedAt) {
		return errors.New("sqlite: endpoint completion time is invalid")
	}
	if err := instant.Validate(outcome.FinishedAt); err != nil {
		return err
	}
	state := modelInvocationUnknown.databaseValue()
	var response *string
	var encodedUsage *string
	var usage *chat.Usage
	if outcome.Response != nil {
		if err := outcome.Response.Validate(); err != nil {
			return err
		}
		data, err := encodeStoredJSON(outcome.Response)
		if err != nil {
			return err
		}
		input, err := jsoninput.Parse(data)
		if err != nil {
			return err
		}
		response = new(input.String())
		if outcome.Response.Metadata != nil {
			usage = outcome.Response.Metadata.Usage
		}
		state = modelInvocationCompleted.databaseValue()
		if outcome.Failure != "" {
			state = modelInvocationFailed.databaseValue()
		}
	}
	encodedUsage, err := encodeModelInvocationUsage(usage)
	if err != nil {
		return err
	}
	encoded, err := encodeEndpointInvocation(invocation)
	if err != nil {
		return err
	}
	result, err := Connection(ctx, m.db).ExecContext(ctx, `UPDATE model_invocations SET state=?,finished_at=?,known_response=?,usage=?,endpoint_failure=?,endpoint_cost_usd=?
		WHERE call_id=? AND state=? AND started_at=? AND endpoint_json=?`, state, outcome.FinishedAt.UTC().UnixNano(), response, encodedUsage, outcome.Failure, outcome.CostUSD,
		invocation.ID, modelInvocationStarted.databaseValue(), invocation.StartedAt.UTC().UnixNano(), string(encoded))
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
	rows, err := Connection(ctx, m.db).QueryContext(ctx, query, args...)
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
	var value modelref.EndpointObservation
	var encoded string
	var id string
	var startedAt int64
	var finishedAt *int64
	var response *string
	var encodedUsage *string
	if err := row.Scan(&id, &startedAt, &encoded, &value.State, &finishedAt, &response, &value.Failure, &value.SessionID, &value.RunID, &value.SegmentID, &value.CostUSD, &encodedUsage); err != nil {
		return modelref.EndpointObservation{}, err
	}
	value.InvocationJSON, err = jsoninput.Parse([]byte(encoded))
	if err != nil {
		return modelref.EndpointObservation{}, err
	}
	var admission storedEndpointInvocation
	if err := decodeStoredJSON([]byte(encoded), &admission); err != nil {
		return modelref.EndpointObservation{}, err
	}
	value.Invocation = modelref.EndpointInvocation{ID: admission.ID, Caller: admission.Caller, Provider: admission.Provider, Model: admission.Model, SessionID: admission.SessionID, Request: admission.Request, StartedAt: admission.StartedAt}
	var fields map[string]jsontext.Value
	if err := decodeStoredJSON([]byte(encoded), &fields); err != nil {
		return modelref.EndpointObservation{}, err
	}
	value.RequestJSON, err = jsoninput.Parse(fields["request"])
	if err != nil {
		return modelref.EndpointObservation{}, err
	}
	if err := value.Invocation.Validate(); err != nil {
		return modelref.EndpointObservation{}, err
	}
	if value.Invocation.ID != id || value.Invocation.StartedAt.UTC().UnixNano() != startedAt || (parent != "" && value.Invocation.Caller.ID != parent && value.Invocation.Caller.Parent != parent) {
		return modelref.EndpointObservation{}, errors.New("sqlite: endpoint observation identity differs from its row")
	}
	if value.Invocation.SessionID != "" && value.Invocation.SessionID != value.SessionID {
		return modelref.EndpointObservation{}, errors.New("sqlite: endpoint Session differs from its admission")
	}
	if value.SessionID != "" {
		if err := resourceid.ValidateSession(value.SessionID); err != nil {
			return modelref.EndpointObservation{}, err
		}
	}
	if (value.RunID == "") != (value.SegmentID == "") {
		return modelref.EndpointObservation{}, errors.New("sqlite: endpoint execution association is incomplete")
	}
	if value.RunID != "" {
		if value.SessionID == "" {
			return modelref.EndpointObservation{}, errors.New("sqlite: endpoint Run has no Session")
		}
		if err := resourceid.ValidateRun(value.RunID); err != nil {
			return modelref.EndpointObservation{}, err
		}
		if err := resourceid.ValidateSegment(value.SegmentID); err != nil {
			return modelref.EndpointObservation{}, err
		}
	}
	if response != nil {
		value.ResponseJSON, err = jsoninput.Parse([]byte(*response))
		if err != nil {
			return modelref.EndpointObservation{}, err
		}
		if err := decodeStoredJSON([]byte(*response), &value.Response); err != nil {
			return modelref.EndpointObservation{}, err
		}
		if value.Response == nil || value.Response.Validate() != nil || (value.State != modelInvocationCompleted.databaseValue() && value.State != modelInvocationFailed.databaseValue()) {
			return modelref.EndpointObservation{}, fmt.Errorf("sqlite: endpoint response evidence is invalid")
		}
	}
	var usage *chat.Usage
	if encodedUsage != nil {
		usage, err = decodeModelInvocationUsage(*encodedUsage)
		if err != nil {
			return modelref.EndpointObservation{}, err
		}
	}
	var reported *chat.Usage
	if value.Response != nil && value.Response.Metadata != nil {
		reported = value.Response.Metadata.Usage
	}
	if !reflect.DeepEqual(usage, reported) {
		return modelref.EndpointObservation{}, errors.New("sqlite: endpoint usage differs from known response")
	}
	if _, err := accounting.CostFromOptional(value.CostUSD); err != nil {
		return modelref.EndpointObservation{}, err
	}
	if value.CostUSD != nil && (value.Response == nil || value.Response.Metadata == nil || value.Response.Metadata.Usage == nil) {
		return modelref.EndpointObservation{}, errors.New("sqlite: endpoint price has no reported usage")
	}
	if finishedAt != nil {
		value.FinishedAt = time.Unix(0, *finishedAt).UTC()
		if value.FinishedAt.Before(value.Invocation.StartedAt) {
			return modelref.EndpointObservation{}, errors.New("sqlite: endpoint settlement precedes admission")
		}
	}
	switch value.State {
	case modelInvocationStarted.databaseValue():
		if finishedAt != nil || response != nil || value.Failure != "" {
			return modelref.EndpointObservation{}, errors.New("sqlite: started endpoint carries completion")
		}
	case modelInvocationCompleted.databaseValue():
		if response == nil || finishedAt == nil || value.Failure != "" {
			return modelref.EndpointObservation{}, errors.New("sqlite: completed endpoint is missing its outcome")
		}
	case modelInvocationFailed.databaseValue():
		if response == nil || finishedAt == nil || value.Failure == "" {
			return modelref.EndpointObservation{}, errors.New("sqlite: failed endpoint is missing known evidence")
		}
	case modelInvocationUnknown.databaseValue():
		if response != nil || finishedAt == nil || value.Failure == "" {
			return modelref.EndpointObservation{}, errors.New("sqlite: unknown endpoint asserts completion")
		}
	default:
		return modelref.EndpointObservation{}, errors.New("sqlite: unknown endpoint state")
	}
	return value, nil
}

func (m *ModelInvocationStore) ReconcileEndpointInvocations(ctx context.Context, settledAt time.Time) error {
	if err := instant.Validate(settledAt); err != nil {
		return err
	}
	_, err := Connection(ctx, m.db).ExecContext(ctx, `UPDATE model_invocations SET state=?,finished_at=MAX(started_at,?),endpoint_failure='process stopped before endpoint completion was recorded' WHERE endpoint_json IS NOT NULL AND state=?`, modelInvocationUnknown.databaseValue(), settledAt.UTC().UnixNano(), modelInvocationStarted.databaseValue())
	return err
}

func (m *ModelInvocationStore) EndpointForCaller(ctx context.Context, caller string) (modelref.EndpointObservation, bool, error) {
	row := Connection(ctx, m.db).QueryRowContext(ctx, `SELECT `+endpointObservationColumns+` FROM model_invocations WHERE endpoint_json IS NOT NULL AND json_extract(endpoint_json,'$.caller.id')=?`, caller)
	value, err := scanEndpointObservation(row, caller)
	if errors.Is(err, sql.ErrNoRows) {
		return modelref.EndpointObservation{}, false, nil
	}
	return value, err == nil, err
}

// ListSessionEndpoints materializes evidence after the export caller checks
// source limits in the same transaction. Ordinary inspection uses bounded pages.
func (m *ModelInvocationStore) ListSessionEndpoints(ctx context.Context, sessionID string) ([]modelref.EndpointObservation, error) {
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return nil, err
	}
	rows, err := Connection(ctx, m.db).QueryContext(ctx, `SELECT `+endpointObservationColumns+` FROM model_invocations WHERE endpoint_json IS NOT NULL AND session_id=? ORDER BY started_at,call_id`, sessionID)
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
	row := Connection(ctx, m.db).QueryRowContext(ctx, `SELECT `+endpointObservationColumns+` FROM model_invocations WHERE endpoint_json IS NOT NULL AND session_id=? AND call_id=?`, sessionID, id)
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
	if err := Connection(ctx, m.db).QueryRowContext(ctx, `SELECT logical_purpose FROM model_invocations WHERE call_id=? AND endpoint_json IS NULL`, callID).Scan(&actualPurpose); err != nil {
		return err
	}
	if actualPurpose != purpose.String() {
		return errors.New("sqlite: recovered model purpose changed")
	}
	if source != nil {
		if source.RunID != runID || source.SessionID != sessionID || source.SegmentID != segmentID || source.Invocation.Caller.ID != callID || source.Invocation.StartedAt.Before(startedAt) || source.FinishedAt.After(finishedAt) {
			return errors.New("sqlite: recovered endpoint differs from original call")
		}
		if err := m.validateEndpointProjection(ctx, *source); err != nil {
			return err
		}
	}
	if source == nil || source.Response == nil {
		return m.MarkModelInvocationUnknown(ctx, sessionID, runID, segmentID, callID, startedAt, finishedAt)
	}
	if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
		return err
	}
	if err := instant.Validate(startedAt, finishedAt); err != nil {
		return err
	}
	if err := source.ResponseJSON.Validate(); err != nil {
		return err
	}
	if err := source.InvocationJSON.Validate(); err != nil {
		return err
	}
	var reported *chat.Usage
	if source.Response != nil && source.Response.Metadata != nil {
		reported = source.Response.Metadata.Usage
	}
	usage, err := encodeModelInvocationUsage(reported)
	if err != nil {
		return err
	}
	result, err := Connection(ctx, m.db).ExecContext(ctx, `WITH evidence AS (
	 SELECT usage,known_response FROM model_invocations WHERE call_id=? AND endpoint_json=? AND session_id=? AND run_id=? AND segment_id=?
	 AND state=? AND finished_at=? AND known_response=? AND endpoint_cost_usd IS ? AND endpoint_failure=? AND usage IS ?)
	 UPDATE model_invocations SET state=?,finished_at=?,usage=(SELECT usage FROM evidence),known_response=(SELECT known_response FROM evidence)
	 WHERE call_id=? AND session_id=? AND run_id=? AND segment_id=? AND state=? AND started_at=? AND endpoint_json IS NULL AND EXISTS(SELECT 1 FROM evidence)`,
		source.Invocation.ID, source.InvocationJSON.String(), sessionID, runID, segmentID, source.State, source.FinishedAt.UTC().UnixNano(), source.ResponseJSON.String(), source.CostUSD, source.Failure, usage,
		modelInvocationFailed.databaseValue(), finishedAt.UTC().UnixNano(), callID, sessionID, runID, segmentID, modelInvocationStarted.databaseValue(), startedAt.UTC().UnixNano())
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
		if err := resourceid.ValidateSession(sessionID); err != nil {
			return err
		}
	}
	if !since.IsZero() {
		if err := instant.Validate(since); err != nil {
			return err
		}
	}
	return RunInTx(ctx, m.db, func(ctx context.Context) error {
		if sessionID != "" {
			var exists bool
			if err := Connection(ctx, m.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=?)`, sessionID).Scan(&exists); err != nil {
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
		if err := Connection(ctx, m.db).QueryRowContext(ctx, ownership, ownershipArgs...).Scan(&invalid); err != nil {
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
		rows, err := Connection(ctx, m.db).QueryContext(ctx, query, args...)
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
	rows, err := Connection(ctx, m.db).QueryContext(ctx, `SELECT `+endpointObservationColumns+` FROM model_invocations
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
	source, found, err := m.EndpointForCaller(ctx, expected.Invocation.Caller.ID)
	if err != nil {
		return err
	}
	if !found || source.InvocationJSON != expected.InvocationJSON || source.RequestJSON != expected.RequestJSON || source.ResponseJSON != expected.ResponseJSON || source.SessionID != expected.SessionID || source.RunID != expected.RunID || source.SegmentID != expected.SegmentID || source.State != expected.State || source.FinishedAt != expected.FinishedAt || source.Failure != expected.Failure || !reflect.DeepEqual(source.CostUSD, expected.CostUSD) || source.RunID == "" || (source.Response == nil) != (expected.Response == nil) {
		return errors.New("sqlite: independent endpoint recovery source changed")
	}

	if source.Invocation.ID != expected.Invocation.ID || source.Invocation.Caller != expected.Invocation.Caller || source.Invocation.Provider != expected.Invocation.Provider || source.Invocation.Model != expected.Invocation.Model || source.Invocation.SessionID != expected.Invocation.SessionID || !source.Invocation.StartedAt.Equal(expected.Invocation.StartedAt) {
		return errors.New("sqlite: independent endpoint admission projection changed")
	}
	var actualUsage, expectedUsage *chat.Usage
	actualModel, expectedModel := source.Invocation.Model, expected.Invocation.Model
	if source.Response != nil && source.Response.Metadata != nil {
		actualUsage = source.Response.Metadata.Usage
		if source.Response.Metadata.Model != "" {
			actualModel = source.Response.Metadata.Model
		}
	}
	if expected.Response != nil && expected.Response.Metadata != nil {
		expectedUsage = expected.Response.Metadata.Usage
		if expected.Response.Metadata.Model != "" {
			expectedModel = expected.Response.Metadata.Model
		}
	}
	if actualModel != expectedModel || !reflect.DeepEqual(actualUsage, expectedUsage) {
		return errors.New("sqlite: independent endpoint consumption projection changed")
	}
	return nil
}

func (m *ModelInvocationStore) ValidateIndependentEndpoint(ctx context.Context, expected modelref.EndpointObservation) error {
	if err := m.validateEndpointProjection(ctx, expected); err != nil {
		return err
	}

	var logical int
	if err := Connection(ctx, m.db).QueryRowContext(ctx, `SELECT count(*) FROM model_invocations WHERE call_id=? AND endpoint_json IS NULL`, expected.Invocation.Caller.ID).Scan(&logical); err != nil {
		return err
	}
	if logical != 0 {
		return errors.New("sqlite: independent endpoint has a logical accounting owner")
	}
	return nil
}
