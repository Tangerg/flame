package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

type ToolInvocationStore struct{ db *database }

func newToolInvocationStore(db *database) *ToolInvocationStore {
	return &ToolInvocationStore{db: db}
}

func (t *ToolInvocationStore) ListOpenToolInvocations(ctx context.Context) ([]runs.OpenToolInvocation, error) {
	return scopedValue(ctx, t.db.read, func(ctx context.Context) ([]runs.OpenToolInvocation, error) {
		rows, err := t.db.connection(ctx).QueryContext(ctx, `
		SELECT session_id, run_id, segment_id, call_id, item_id, started_at
		  FROM tool_invocations
		 WHERE state = ?
		 ORDER BY session_id, run_id, segment_id, call_id, item_id
	`, string(runs.ToolInvocationStarted))
		if err != nil {
			return nil, fmt.Errorf("sqlite: list started Tool invocations: %w", err)
		}
		defer rows.Close()
		var open []runs.OpenToolInvocation
		for rows.Next() {
			var invocation runs.OpenToolInvocation
			var startedAt int64
			if err := rows.Scan(&invocation.SessionID, &invocation.RunID, &invocation.SegmentID, &invocation.CallID, &invocation.ItemID, &startedAt); err != nil {
				return nil, fmt.Errorf("sqlite: scan started Tool invocation: %w", err)
			}
			invocation.StartedAt = time.Unix(0, startedAt).UTC()
			if err := invocation.Validate(); err != nil {
				return nil, fmt.Errorf("sqlite: restore started Tool invocation: %w", err)
			}
			open = append(open, invocation)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: iterate started Tool invocations: %w", err)
		}
		return open, nil
	})
}

func (t *ToolInvocationStore) RecordToolInvocation(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ToolInvocationCommit) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	if err := validateRunCoordinates("Tool invocation", sessionID, runID, invocation.SegmentID); err != nil {
		return err
	}
	switch invocation.State {
	case runs.ToolInvocationStarted:
		return t.start(ctx, sessionID, runID, invocation)
	case runs.ToolInvocationCompleted:
		return t.complete(ctx, sessionID, runID, invocation)
	default:
		return t.finish(ctx, sessionID, runID, invocation)
	}
}

func (t *ToolInvocationStore) start(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ToolInvocationCommit) error {
	if err := instant.Validate(invocation.StartedAt); err != nil {
		return fmt.Errorf("sqlite: Tool invocation start time: %w", err)
	}
	return t.db.write(ctx, func(ctx context.Context) error {
		if invocation.Caller != nil {
			if err := t.retainCaller(ctx, *invocation.Caller); err != nil {
				return err
			}
		}
		result, err := t.db.connection(ctx).ExecContext(ctx, `INSERT INTO tool_invocations(
		   call_id, item_id, session_id, run_id, segment_id, state, started_at, finished_at)
		 SELECT ?, ?, session_id, run_id, ?, ?, ?, NULL
		   FROM runs
		  WHERE run_id = ? AND session_id = ? AND state = ? AND active_segment_id = ?`, invocation.CallID, invocation.ItemID, invocation.SegmentID, string(runs.ToolInvocationStarted), invocation.StartedAt.UTC().UnixNano(), runID, sessionID, string(rundomain.Running), invocation.SegmentID)
		if err != nil {
			return fmt.Errorf("sqlite: start Tool invocation %q: %w", invocation.CallID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect Tool invocation %q start: %w", invocation.CallID, err)
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: Tool invocation %q has no owning active Run", invocation.CallID)
		}
		return nil
	})
}

func (t *ToolInvocationStore) complete(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ToolInvocationCommit) error {
	return t.db.write(ctx, func(ctx context.Context) error {
		if err := instant.Validate(invocation.StartedAt, invocation.FinishedAt); err != nil {
			return fmt.Errorf("sqlite: Tool invocation times: %w", err)
		}
		result, err := t.db.connection(ctx).ExecContext(ctx, `
		INSERT INTO tool_invocations(call_id, item_id, session_id, run_id, segment_id, state, started_at, finished_at)
		SELECT ?, ?, session_id, run_id, ?, ?, ?, ? FROM runs
		WHERE session_id = ? AND run_id = ? AND state = ? AND active_segment_id = ?
		ON CONFLICT(call_id, segment_id) DO UPDATE SET state = excluded.state, finished_at = excluded.finished_at
		WHERE tool_invocations.state = ? AND tool_invocations.item_id = excluded.item_id
		AND tool_invocations.session_id = excluded.session_id AND tool_invocations.run_id = excluded.run_id
		AND tool_invocations.started_at = excluded.started_at`, invocation.CallID, invocation.ItemID, invocation.SegmentID, string(runs.ToolInvocationCompleted), invocation.StartedAt.UTC().UnixNano(), invocation.FinishedAt.UTC().UnixNano(), sessionID, runID, string(rundomain.Running), invocation.SegmentID, string(runs.ToolInvocationStarted))
		if err != nil {
			return fmt.Errorf("sqlite: complete Tool invocation %q: %w", invocation.CallID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: Tool invocation %q lost its completion ownership", invocation.CallID)
		}
		return nil
	})
}

func (t *ToolInvocationStore) finish(ctx context.Context, sessionID identity.SessionID, runID identity.RunID, invocation runs.ToolInvocationCommit) error {
	return t.db.write(ctx, func(ctx context.Context) error {
		if err := instant.Validate(invocation.StartedAt, invocation.FinishedAt); err != nil {
			return fmt.Errorf("sqlite: Tool invocation times: %w", err)
		}
		result, err := t.db.connection(ctx).ExecContext(ctx, `UPDATE tool_invocations
		    SET state = ?, finished_at = ?
		  WHERE call_id = ? AND item_id = ? AND session_id = ? AND run_id = ? AND segment_id = ?
		    AND state = ? AND started_at = ?`, string(invocation.State), invocation.FinishedAt.UTC().UnixNano(), invocation.CallID, invocation.ItemID, sessionID, runID, invocation.SegmentID, string(runs.ToolInvocationStarted), invocation.StartedAt.UTC().UnixNano())
		if err != nil {
			return fmt.Errorf("sqlite: finish Tool invocation %q as %s: %w", invocation.CallID, invocation.State, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect Tool invocation %q transition: %w", invocation.CallID, err)
		}
		if changed != 1 {
			return fmt.Errorf("sqlite: Tool invocation %q no longer owns its started transition", invocation.CallID)
		}
		return nil
	})
}

func validateToolInvocationIdentity(sessionID identity.SessionID, runID identity.RunID, segmentID identity.SegmentID, callID string, itemID identity.ItemID) error {
	if err := validateRunCoordinates("Tool invocation", sessionID, runID, segmentID); err != nil {
		return err
	}
	if err := validateResource("Tool invocation", itemID); err != nil {
		return err
	}
	if err := runs.ValidateInvocation(callID); err != nil {
		return fmt.Errorf("sqlite: Tool invocation: %w", err)
	}
	return nil
}

type toolInvocationRecord struct {
	CallID     string
	ItemID     identity.ItemID
	RunID      identity.RunID
	SegmentID  identity.SegmentID
	State      string
	StartedAt  time.Time
	FinishedAt time.Time
}

func (t *ToolInvocationStore) ListSession(ctx context.Context, sessionID identity.SessionID) ([]toolInvocationRecord, error) {
	return scopedValue(ctx, t.db.read, func(ctx context.Context) ([]toolInvocationRecord, error) {
		if err := validateResource("tool invocation", sessionID); err != nil {
			return nil, err
		}
		rows, err := t.db.connection(ctx).QueryContext(ctx, `
		SELECT call_id, item_id, run_id, segment_id, state, started_at, finished_at
		  FROM tool_invocations
		 WHERE session_id = ?
		 ORDER BY started_at, call_id, segment_id`, sessionID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list session tool invocations: %w", err)
		}
		defer rows.Close()
		var records []toolInvocationRecord
		for rows.Next() {
			var record toolInvocationRecord
			var startedAt int64
			var finishedAt sql.NullInt64
			if err := rows.Scan(&record.CallID, &record.ItemID, &record.RunID, &record.SegmentID, &record.State, &startedAt, &finishedAt); err != nil {
				return nil, fmt.Errorf("sqlite: scan session tool invocation: %w", err)
			}
			if err := validateToolInvocationIdentity(sessionID, record.RunID, record.SegmentID, record.CallID, record.ItemID); err != nil {
				return nil, fmt.Errorf("sqlite: restore tool invocation: %w", err)
			}
			record.StartedAt = time.Unix(0, startedAt).UTC()
			if finishedAt.Valid {
				record.FinishedAt = time.Unix(0, finishedAt.Int64).UTC()
			}
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: iterate session tool invocations: %w", err)
		}
		return records, nil
	})
}
