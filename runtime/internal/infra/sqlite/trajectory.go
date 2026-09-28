package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type TrajectoryPosition struct {
	OccurredAt int64
	Kind       string
	ID         string
}

type trajectoryIdentity struct {
	TrajectoryPosition
	runID string
}

type TrajectoryRecord struct {
	OccurredAt time.Time
	RunID      string
	Run        *run.Run
	Model      *ModelInvocationRecord
	Item       *transcript.Item
}

type TrajectoryStore struct {
	db         *sql.DB
	runs       *RunStore
	transcript *TranscriptStore
	models     *ModelInvocationStore
}

func NewTrajectoryStore(db *sql.DB) *TrajectoryStore {
	return &TrajectoryStore{db: db, runs: NewRunStore(db), transcript: NewTranscriptStore(db), models: NewModelInvocationStore(db)}
}

func (s *TrajectoryStore) Page(ctx context.Context, sessionID string, includeDescendants bool, anchor *TrajectoryPosition, limit int) ([]TrajectoryRecord, error) {
	if err := resourceid.ValidateSession(sessionID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, errors.New("sqlite: trajectory page requires a positive limit")
	}
	var result []TrajectoryRecord
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		var exists bool
		if err := conn(ctx, s.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`, sessionID).Scan(&exists); err != nil {
			return fmt.Errorf("sqlite: trajectory Session: %w", err)
		}
		if !exists {
			return session.ErrNotFound
		}
		positions, err := s.positions(ctx, sessionID, includeDescendants, anchor, limit)
		if err != nil {
			return err
		}
		result = make([]TrajectoryRecord, 0, len(positions))
		for _, position := range positions {
			entry, err := s.hydrate(ctx, sessionID, position)
			if err != nil {
				return err
			}
			result = append(result, entry)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *TrajectoryStore) positions(ctx context.Context, sessionID string, includeDescendants bool, anchor *TrajectoryPosition, limit int) ([]trajectoryIdentity, error) {
	query := `SELECT occurred_at, kind, id, run_id FROM (
		SELECT created_at AS occurred_at, 'run' AS kind, run_id AS id, run_id FROM runs WHERE session_id = ?
		UNION ALL
		SELECT started_at, 'model', call_id, run_id FROM model_invocations WHERE session_id = ?
		UNION ALL
		SELECT occurred_at, 'item', item_id, run_id FROM history_items WHERE session_id = ?
	) WHERE 1 = 1`
	args := []any{sessionID, sessionID, sessionID}
	if !includeDescendants {
		query += ` AND run_id IN (SELECT run_id FROM runs WHERE session_id = ? AND root_run_id = '')`
		args = append(args, sessionID)
	}
	if anchor != nil {
		query += ` AND (occurred_at, kind, id) < (?, ?, ?)`
		args = append(args, anchor.OccurredAt, anchor.Kind, anchor.ID)
	}
	query += ` ORDER BY occurred_at DESC, kind DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: page trajectory: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var positions []trajectoryIdentity
	for rows.Next() {
		var position trajectoryIdentity
		if err := rows.Scan(&position.OccurredAt, &position.Kind, &position.ID, &position.runID); err != nil {
			return nil, fmt.Errorf("sqlite: scan trajectory: %w", err)
		}
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate trajectory: %w", err)
	}
	return positions, nil
}

func (s *TrajectoryStore) hydrate(ctx context.Context, sessionID string, position trajectoryIdentity) (TrajectoryRecord, error) {
	entry := TrajectoryRecord{OccurredAt: time.Unix(0, position.OccurredAt).UTC()}
	switch position.Kind {
	case "run":
		value, found, err := s.runs.Run(ctx, position.ID)
		if err != nil {
			return TrajectoryRecord{}, err
		}
		if !found || value.SessionID() != sessionID {
			return TrajectoryRecord{}, errors.New("sqlite: trajectory Run has no owning Session")
		}
		entry.Run, entry.RunID = &value, value.ID()
	case "model":
		value, err := s.models.trajectoryInvocation(ctx, sessionID, position.runID, position.ID)
		if err != nil {
			return TrajectoryRecord{}, err
		}
		entry.Model, entry.RunID = &value, position.runID
	case "item":
		value, found, err := s.transcript.Item(ctx, position.ID)
		if err != nil {
			return TrajectoryRecord{}, err
		}
		if !found || value.SessionID() != sessionID {
			return TrajectoryRecord{}, errors.New("sqlite: trajectory Item has no owning Session")
		}
		entry.Item, entry.RunID = &value, value.RunID()
	default:
		return TrajectoryRecord{}, errors.New("sqlite: unknown trajectory source")
	}
	if entry.Run == nil {
		owner, found, err := s.runs.Run(ctx, entry.RunID)
		if err != nil {
			return TrajectoryRecord{}, err
		}
		if !found || owner.SessionID() != sessionID {
			return TrajectoryRecord{}, errors.New("sqlite: trajectory source has no owning Session Run")
		}
	}
	return entry, nil
}

func (m *ModelInvocationStore) trajectoryInvocation(ctx context.Context, sessionID, runID, callID string) (ModelInvocationRecord, error) {
	row := conn(ctx, m.db).QueryRowContext(ctx, `SELECT call_id, segment_id, state, started_at, finished_at, usage, first_output_latency_millis FROM model_invocations WHERE session_id = ? AND run_id = ? AND call_id = ?`, sessionID, runID, callID)
	record, err := scanModelInvocation(row)
	if err != nil {
		return ModelInvocationRecord{}, err
	}
	if err := validateModelInvocationIdentity(sessionID, runID, record.SegmentID, record.CallID); err != nil {
		return ModelInvocationRecord{}, err
	}
	return record, nil
}
