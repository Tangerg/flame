package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type trajectoryStore struct {
	db         *database
	runs       *RunStore
	transcript *TranscriptStore
	models     *ModelInvocationStore
}

func newTrajectoryStore(db *database) *trajectoryStore {
	return &trajectoryStore{db: db, runs: newRunStore(db), transcript: newTranscriptStore(db), models: newModelInvocationStore(db)}
}

type trajectoryRow struct {
	sessions.TrajectoryPosition
	runID identity.RunID
}

var trajectoryPositionsQuery = fmt.Sprintf(`SELECT occurred_at, kind, id, run_id FROM (
		SELECT created_at AS occurred_at, '%s' AS kind, run_id AS id, run_id FROM runs WHERE session_id = ?
		UNION ALL
		SELECT started_at, '%s', call_id, run_id FROM model_invocations WHERE session_id = ? AND endpoint_json IS NULL
		UNION ALL
		SELECT started_at, '%s', call_id, COALESCE(run_id,'') FROM model_invocations WHERE session_id = ? AND endpoint_json IS NOT NULL
		UNION ALL
		SELECT occurred_at, '%s', item_id, run_id FROM history_items WHERE session_id = ?
	) WHERE 1 = 1`,
	sessions.TrajectoryKindRun, sessions.TrajectoryKindModel, sessions.TrajectoryKindEndpoint, sessions.TrajectoryKindItem)

func (s *trajectoryStore) PageTrajectory(ctx context.Context, sessionID identity.SessionID, includeDescendants bool, anchor *sessions.TrajectoryPosition, limit int) ([]sessions.TrajectoryEntry, error) {
	if err := sessionID.Validate(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, errors.New("sqlite: trajectory page requires a positive limit")
	}
	var entries []sessions.TrajectoryEntry
	err := s.db.read(ctx, func(ctx context.Context) error {
		var exists bool
		if err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`, sessionID).Scan(&exists); err != nil {
			return fmt.Errorf("sqlite: trajectory Session: %w", err)
		}
		if !exists {
			return session.ErrNotFound
		}
		rows, err := s.positions(ctx, sessionID, includeDescendants, anchor, limit)
		if err != nil {
			return err
		}
		entries = make([]sessions.TrajectoryEntry, 0, len(rows))
		for _, row := range rows {
			source, err := s.source(ctx, sessionID, row)
			if err != nil {
				return err
			}
			entries = append(entries, sessions.TrajectoryEntry{OccurredAt: time.Unix(0, row.OccurredAt).UTC(), Source: source})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *trajectoryStore) positions(ctx context.Context, sessionID identity.SessionID, includeDescendants bool, anchor *sessions.TrajectoryPosition, limit int) ([]trajectoryRow, error) {
	query := trajectoryPositionsQuery
	args := []any{sessionID, sessionID, sessionID, sessionID}
	if !includeDescendants {
		query += ` AND (run_id = '' OR run_id IN (SELECT run_id FROM runs WHERE session_id = ? AND root_run_id = ''))`
		args = append(args, sessionID)
	}
	if anchor != nil {
		query += ` AND (occurred_at, kind, id) < (?, ?, ?)`
		args = append(args, anchor.OccurredAt, string(anchor.Kind), anchor.ID)
	}
	query += ` ORDER BY occurred_at DESC, kind DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.connection(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: page trajectory: %w", err)
	}
	defer rows.Close()
	var positions []trajectoryRow
	for rows.Next() {
		var row trajectoryRow
		var kind string
		if err := rows.Scan(&row.OccurredAt, &kind, &row.ID, &row.runID); err != nil {
			return nil, fmt.Errorf("sqlite: scan trajectory: %w", err)
		}
		row.Kind = sessions.TrajectoryKind(kind)
		positions = append(positions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate trajectory: %w", err)
	}
	return positions, nil
}

func (s *trajectoryStore) source(ctx context.Context, sessionID identity.SessionID, row trajectoryRow) (sessions.TrajectorySource, error) {
	switch row.Kind {
	case sessions.TrajectoryKindRun:
		return s.runSource(ctx, row.ID)
	case sessions.TrajectoryKindModel:
		value, err := s.models.trajectoryInvocation(ctx, row.runID, row.ID)
		if err != nil {
			return nil, err
		}
		return sessions.TrajectoryModel{RunID: row.runID, Invocation: value}, nil
	case sessions.TrajectoryKindEndpoint:
		value, err := s.models.trajectoryEndpoint(ctx, sessionID, row.ID)
		if err != nil {
			return nil, err
		}
		return sessions.TrajectoryEndpoint{Observation: value}, nil
	case sessions.TrajectoryKindItem:
		return s.itemSource(ctx, sessionID, row.ID)
	default:
		return nil, fmt.Errorf("sqlite: unknown trajectory kind %q", row.Kind)
	}
}

func (s *trajectoryStore) runSource(ctx context.Context, id string) (sessions.TrajectorySource, error) {
	runID, err := identity.ParseRun(id)
	if err != nil {
		return nil, fmt.Errorf("sqlite: trajectory Run: %w", err)
	}
	value, found, err := s.runs.Run(ctx, runID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("sqlite: trajectory Run vanished from its snapshot")
	}
	return sessions.TrajectoryRun{Run: value}, nil
}

func (s *trajectoryStore) itemSource(ctx context.Context, sessionID identity.SessionID, id string) (sessions.TrajectorySource, error) {
	itemID, err := identity.ParseItem(id)
	if err != nil {
		return nil, fmt.Errorf("sqlite: trajectory Item: %w", err)
	}
	value, found, err := s.transcript.Item(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("sqlite: trajectory Item vanished from its snapshot")
	}
	owner, found, err := s.runs.Run(ctx, value.RunID())
	if err != nil {
		return nil, err
	}
	if !found || owner.SessionID() != sessionID {
		return nil, errors.New("sqlite: trajectory Item has no owning Session Run")
	}
	return sessions.TrajectoryItem{Item: value}, nil
}

func (m *ModelInvocationStore) trajectoryInvocation(ctx context.Context, runID identity.RunID, callID string) (runs.ModelInvocationCommit, error) {
	return scopedValue(ctx, m.db.read, func(ctx context.Context) (runs.ModelInvocationCommit, error) {
		row := m.db.connection(ctx).QueryRowContext(ctx, `SELECT `+modelInvocationColumns+` FROM model_invocations WHERE run_id = ? AND call_id = ? AND endpoint_json IS NULL`, runID, callID)
		return scanModelInvocation(row)
	})
}
