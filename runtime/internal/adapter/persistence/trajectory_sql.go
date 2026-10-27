package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type trajectoryPosition struct {
	OccurredAt int64
	Kind       string
	ID         string
}

type trajectoryIdentity struct {
	trajectoryPosition
	runID string
}

type trajectoryRecord struct {
	OccurredAt time.Time
	RunID      string
	Run        *run.Run
	Endpoint   *modelref.EndpointObservation
	Model      *modelInvocationRecord
	Item       *transcript.Item
}

type trajectoryStore struct {
	db         *sql.DB
	runs       *RunStore
	transcript *TranscriptStore
	models     *ModelInvocationStore
}

func newTrajectoryStore(db *sql.DB) *trajectoryStore {
	return &trajectoryStore{db: db, runs: newRunStore(db), transcript: newTranscriptStore(db), models: newModelInvocationStore(db)}
}

func (s *trajectoryStore) Page(ctx context.Context, sessionID string, includeDescendants bool, anchor *trajectoryPosition, limit int) ([]trajectoryRecord, error) {
	if err := identity.ValidateSession(sessionID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, errors.New("sqlite: trajectory page requires a positive limit")
	}
	var result []trajectoryRecord
	err := sqlite.RunReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		var exists bool
		if err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`, sessionID).Scan(&exists); err != nil {
			return fmt.Errorf("sqlite: trajectory Session: %w", err)
		}
		if !exists {
			return session.ErrNotFound
		}
		positions, err := s.positions(ctx, sessionID, includeDescendants, anchor, limit)
		if err != nil {
			return err
		}
		result = make([]trajectoryRecord, 0, len(positions))
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

func (s *trajectoryStore) positions(ctx context.Context, sessionID string, includeDescendants bool, anchor *trajectoryPosition, limit int) ([]trajectoryIdentity, error) {
	query := `SELECT occurred_at, kind, id, run_id FROM (
		SELECT created_at AS occurred_at, 'run' AS kind, run_id AS id, run_id FROM runs WHERE session_id = ?
		UNION ALL
		SELECT started_at, 'model', call_id, run_id FROM model_invocations WHERE session_id = ? AND endpoint_json IS NULL
		UNION ALL
		SELECT started_at, 'endpoint', call_id, COALESCE(run_id,'') FROM model_invocations WHERE session_id = ? AND endpoint_json IS NOT NULL
		UNION ALL
		SELECT occurred_at, 'item', item_id, run_id FROM history_items WHERE session_id = ?
	) WHERE 1 = 1`
	args := []any{sessionID, sessionID, sessionID, sessionID}
	if !includeDescendants {
		query += ` AND (run_id = '' OR run_id IN (SELECT run_id FROM runs WHERE session_id = ? AND root_run_id = ''))`
		args = append(args, sessionID)
	}
	if anchor != nil {
		query += ` AND (occurred_at, kind, id) < (?, ?, ?)`
		args = append(args, anchor.OccurredAt, anchor.Kind, anchor.ID)
	}
	query += ` ORDER BY occurred_at DESC, kind DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, query, args...)
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

func (s *trajectoryStore) hydrate(ctx context.Context, sessionID string, position trajectoryIdentity) (trajectoryRecord, error) {
	entry := trajectoryRecord{OccurredAt: time.Unix(0, position.OccurredAt).UTC()}
	switch position.Kind {
	case "run":
		value, found, err := s.runs.Run(ctx, position.ID)
		if err != nil {
			return trajectoryRecord{}, err
		}
		if !found || value.SessionID() != sessionID {
			return trajectoryRecord{}, errors.New("sqlite: trajectory Run has no owning Session")
		}
		entry.Run, entry.RunID = &value, value.ID()
	case "model":
		value, err := s.models.trajectoryInvocation(ctx, sessionID, position.runID, position.ID)
		if err != nil {
			return trajectoryRecord{}, err
		}
		entry.Model, entry.RunID = &value, position.runID
	case "endpoint":
		value, err := s.models.trajectoryEndpoint(ctx, sessionID, position.ID)
		if err != nil {
			return trajectoryRecord{}, err
		}
		entry.Endpoint, entry.RunID = &value, value.RunID
	case "item":
		value, found, err := s.transcript.Item(ctx, position.ID)
		if err != nil {
			return trajectoryRecord{}, err
		}
		if !found || value.SessionID() != sessionID {
			return trajectoryRecord{}, errors.New("sqlite: trajectory Item has no owning Session")
		}
		entry.Item, entry.RunID = &value, value.RunID()
	default:
		return trajectoryRecord{}, errors.New("sqlite: unknown trajectory source")
	}
	if entry.Run == nil && (entry.Endpoint == nil || entry.RunID != "") {
		owner, found, err := s.runs.Run(ctx, entry.RunID)
		if err != nil {
			return trajectoryRecord{}, err
		}
		if !found || owner.SessionID() != sessionID {
			return trajectoryRecord{}, errors.New("sqlite: trajectory source has no owning Session Run")
		}
	}
	return entry, nil
}

func (m *ModelInvocationStore) trajectoryInvocation(ctx context.Context, sessionID, runID, callID string) (modelInvocationRecord, error) {
	row := sqlite.Connection(ctx, m.db).QueryRowContext(ctx, `SELECT call_id, segment_id, state, started_at, finished_at, usage, first_output_latency_millis, known_response FROM model_invocations WHERE session_id = ? AND run_id = ? AND call_id = ?`, sessionID, runID, callID)
	record, err := scanModelInvocation(row)
	if err != nil {
		return modelInvocationRecord{}, err
	}
	if err := validateModelInvocationIdentity(sessionID, runID, record.SegmentID, record.CallID); err != nil {
		return modelInvocationRecord{}, err
	}
	return record, nil
}
