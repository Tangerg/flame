package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/instant"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/scope/core/chat"
)

const endpointObservationColumns = `call_id,started_at,endpoint_json,state,finished_at,known_response,endpoint_failure,COALESCE(session_id,''),COALESCE(run_id,''),endpoint_cost_usd,usage`

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

func (m *ModelInvocationStore) StartEndpointInvocation(ctx context.Context, invocation modelref.EndpointInvocation) error {
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
	result, err := Connection(ctx, m.db).ExecContext(ctx, `INSERT INTO model_invocations(call_id,session_id,run_id,segment_id,state,started_at,endpoint_json)
		SELECT ?,scope.session_id,scope.run_id,scope.segment_id,?,?,? FROM (
		SELECT COALESCE((SELECT session_id FROM model_invocations WHERE call_id=? AND endpoint_json IS NULL),?) session_id,
		(SELECT run_id FROM model_invocations WHERE call_id=? AND endpoint_json IS NULL) run_id,
		(SELECT segment_id FROM model_invocations WHERE call_id=? AND endpoint_json IS NULL) segment_id) scope
		WHERE (scope.session_id IS NULL OR EXISTS(SELECT 1 FROM sessions WHERE id=scope.session_id))
		AND (? IS NULL OR scope.session_id=?)`,
		invocation.ID, modelInvocationStarted.databaseValue(), invocation.StartedAt.UTC().UnixNano(), string(encoded),
		invocation.Caller.ID, session, invocation.Caller.ID, invocation.Caller.ID, session, session)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("sqlite: endpoint has no matching owning Session")
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
	if err := row.Scan(&id, &startedAt, &encoded, &value.State, &finishedAt, &response, &value.Failure, &value.SessionID, &value.RunID, &value.CostUSD, &encodedUsage); err != nil {
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
	if value.RunID != "" {
		if value.SessionID == "" {
			return modelref.EndpointObservation{}, errors.New("sqlite: endpoint Run has no Session")
		}
		if err := resourceid.ValidateRun(value.RunID); err != nil {
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
func (m *ModelInvocationStore) RecoverModelInvocation(ctx context.Context, sessionID, runID, segmentID, callID string, startedAt, finishedAt time.Time, source *modelref.EndpointObservation) error {
	if source == nil {
		return m.MarkModelInvocationUnknown(ctx, sessionID, runID, segmentID, callID, startedAt, finishedAt)
	}
	if err := validateModelInvocationIdentity(sessionID, runID, segmentID, callID); err != nil {
		return err
	}
	if err := instant.Validate(startedAt, finishedAt); err != nil {
		return err
	}
	if source.RunID != runID || source.SessionID != sessionID || source.Invocation.Caller.ID != callID || source.Response == nil || finishedAt.Before(source.FinishedAt) || source.Invocation.StartedAt.Before(startedAt) {
		return errors.New("sqlite: recovered endpoint differs from original call")
	}
	if err := source.ResponseJSON.Validate(); err != nil {
		return err
	}
	if err := source.InvocationJSON.Validate(); err != nil {
		return err
	}
	var reported *chat.Usage
	if source.Response.Metadata != nil {
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
