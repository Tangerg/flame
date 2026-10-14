package sqlite

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/instant"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/scope/core/chat"
)

func (m *ModelInvocationStore) StartEndpointInvocation(ctx context.Context, invocation modelref.EndpointInvocation) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	if err := instant.Validate(invocation.StartedAt); err != nil {
		return err
	}
	encoded, err := encodeStoredJSON(invocation)
	if err != nil {
		return err
	}
	var session any
	if invocation.SessionID != "" {
		session = invocation.SessionID
	}
	_, err = Connection(ctx, m.db).ExecContext(ctx, `INSERT INTO model_invocations(call_id,session_id,run_id,segment_id,state,started_at,endpoint_json)
		VALUES(?,COALESCE((SELECT session_id FROM model_invocations WHERE call_id=?),?),
		(SELECT run_id FROM model_invocations WHERE call_id=?),(SELECT segment_id FROM model_invocations WHERE call_id=?),?,?,?)`,
		invocation.ID, invocation.Caller.ID, session, invocation.Caller.ID, invocation.Caller.ID, modelInvocationStarted.databaseValue(), invocation.StartedAt.UTC().UnixNano(), string(encoded))
	return err
}

func (m *ModelInvocationStore) SettleEndpointInvocation(ctx context.Context, invocation modelref.EndpointInvocation, outcome modelref.EndpointOutcome) error {
	if err := invocation.Validate(); err != nil {
		return err
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
	var usage *chat.Usage
	if outcome.Response != nil {
		if err := outcome.Response.Validate(); err != nil {
			return err
		}
		data, err := encodeStoredJSON(outcome.Response)
		if err != nil {
			return err
		}
		response = new(string(data))
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
	encoded, err := encodeStoredJSON(invocation)
	if err != nil {
		return err
	}
	result, err := Connection(ctx, m.db).ExecContext(ctx, `UPDATE model_invocations SET state=?,finished_at=?,known_response=?,usage=?,endpoint_failure=?
		WHERE call_id=? AND state=? AND started_at=? AND endpoint_json=?`, state, outcome.FinishedAt.UTC().UnixNano(), response, encodedUsage, outcome.Failure,
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
	query := `SELECT call_id,started_at,endpoint_json,state,finished_at,known_response,endpoint_failure FROM model_invocations WHERE endpoint_json IS NOT NULL AND (json_extract(endpoint_json,'$.caller.id')=? OR json_extract(endpoint_json,'$.caller.parent')=?)`
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
		var value modelref.EndpointObservation
		var encoded string
		var id string
		var startedAt int64
		var finishedAt *int64
		var response *string
		if err := rows.Scan(&id, &startedAt, &encoded, &value.State, &finishedAt, &response, &value.Failure); err != nil {
			return nil, err
		}
		if err := decodeStoredJSON([]byte(encoded), &value.Invocation); err != nil {
			return nil, err
		}
		var fields map[string]jsontext.Value
		if err := decodeStoredJSON([]byte(encoded), &fields); err != nil {
			return nil, err
		}
		value.RequestJSON, err = jsoninput.Parse(fields["request"])
		if err != nil {
			return nil, err
		}
		if err := value.Invocation.Validate(); err != nil {
			return nil, err
		}
		if value.Invocation.ID != id || value.Invocation.StartedAt.UTC().UnixNano() != startedAt || (value.Invocation.Caller.ID != parent && value.Invocation.Caller.Parent != parent) {
			return nil, errors.New("sqlite: endpoint observation identity differs from its row")
		}
		if response != nil {
			value.ResponseJSON, err = jsoninput.Parse([]byte(*response))
			if err != nil {
				return nil, err
			}
			if err := decodeStoredJSON([]byte(*response), &value.Response); err != nil {
				return nil, err
			}
			if value.Response == nil || value.Response.Validate() != nil || (value.State != modelInvocationCompleted.databaseValue() && value.State != modelInvocationFailed.databaseValue()) {
				return nil, fmt.Errorf("sqlite: endpoint response evidence is invalid")
			}
		}
		if finishedAt != nil {
			value.FinishedAt = time.Unix(0, *finishedAt).UTC()
		}
		switch value.State {
		case modelInvocationStarted.databaseValue():
			if finishedAt != nil || response != nil || value.Failure != "" {
				return nil, errors.New("sqlite: started endpoint carries completion")
			}
		case modelInvocationCompleted.databaseValue():
			if response == nil || finishedAt == nil || value.Failure != "" {
				return nil, errors.New("sqlite: completed endpoint is missing its outcome")
			}
		case modelInvocationFailed.databaseValue():
			if response == nil || finishedAt == nil || value.Failure == "" {
				return nil, errors.New("sqlite: failed endpoint is missing known evidence")
			}
		case modelInvocationUnknown.databaseValue():
			if response != nil || finishedAt == nil || value.Failure == "" {
				return nil, errors.New("sqlite: unknown endpoint asserts completion")
			}
		default:
			return nil, errors.New("sqlite: unknown endpoint state")
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (m *ModelInvocationStore) ReconcileEndpointInvocations(ctx context.Context, settledAt time.Time) error {
	if err := instant.Validate(settledAt); err != nil {
		return err
	}
	_, err := Connection(ctx, m.db).ExecContext(ctx, `UPDATE model_invocations SET state=?,finished_at=MAX(started_at,?),endpoint_failure='process stopped before endpoint completion was recorded' WHERE endpoint_json IS NOT NULL AND state=?`, modelInvocationUnknown.databaseValue(), settledAt.UTC().UnixNano(), modelInvocationStarted.databaseValue())
	return err
}
