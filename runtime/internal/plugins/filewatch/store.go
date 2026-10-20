package main

import (
	"bytes"
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/plugins/filewatch/domain"
	pluginstorage "github.com/Tangerg/flame/runtime/plugin/sqlite"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

const (
	maxPending = 32
	maxEvents  = 256
)

var errStored = errors.New("filewatch: invalid private data")

type store struct{ db *sql.DB }

func openStore(ctx context.Context, path string) (*store, error) {
	initial, err := json.Marshal(domain.Initial().Snapshot())
	if err != nil {
		return nil, err
	}
	db, err := sqlitebase.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		`CREATE TABLE IF NOT EXISTS configuration (id INTEGER PRIMARY KEY CHECK(id=1), snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`,
		`INSERT INTO configuration(id,snapshot) VALUES(1,'` + string(initial) + `')`,
		`CREATE TABLE IF NOT EXISTS runtime_target (id INTEGER PRIMARY KEY CHECK(id=1), namespace TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS events (id TEXT PRIMARY KEY, generation TEXT NOT NULL, sequence INTEGER NOT NULL CHECK(sequence>0), capture TEXT NOT NULL CHECK(json_valid(capture)), accepted INTEGER NOT NULL CHECK(accepted IN (0,1)), UNIQUE(generation,sequence))`,
		`CREATE TRIGGER IF NOT EXISTS immutable_capture BEFORE UPDATE OF id,generation,sequence,capture ON events BEGIN SELECT RAISE(ABORT,'observation capture is immutable'); END`,
		`CREATE TRIGGER IF NOT EXISTS accepted_is_final BEFORE UPDATE OF accepted ON events WHEN OLD.accepted=1 AND NEW.accepted<>1 BEGIN SELECT RAISE(ABORT,'acceptance cannot be withdrawn'); END`,
	})
	if err != nil {
		return nil, err
	}
	s := &store{db: db}
	if _, err := s.configuration(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

func (s *store) configuration(ctx context.Context) (domain.Configuration, error) {
	var raw string
	if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT snapshot FROM configuration WHERE id=1`).Scan(&raw); err != nil {
		return domain.Configuration{}, err
	}
	var snapshot domain.Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return domain.Configuration{}, errors.Join(errStored, err)
	}
	value, err := domain.Restore(snapshot)
	if err != nil {
		return domain.Configuration{}, errors.Join(errStored, err)
	}
	return value, nil
}

func (s *store) configure(ctx context.Context, expected int64, draft *domain.Draft) (domain.Snapshot, error) {
	current, err := s.configuration(ctx)
	if err != nil {
		return domain.Snapshot{}, err
	}
	next, err := current.Replace(expected, draft)
	if err != nil {
		return domain.Snapshot{}, err
	}
	encoded, err := json.Marshal(next.Snapshot())
	if err != nil {
		return domain.Snapshot{}, err
	}
	_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `UPDATE configuration SET snapshot=? WHERE id=1`, string(encoded))
	return next.Snapshot(), err
}

func (s *store) checkNamespace(ctx context.Context, namespace string) error {
	if namespace == "" {
		return errors.New("filewatch: Runtime store namespace is required")
	}
	var original string
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT namespace FROM runtime_target WHERE id=1`).Scan(&original)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if original != namespace {
		return errors.New("filewatch: captured work belongs to another Runtime store")
	}
	return nil
}

func (s *store) bindNamespace(ctx context.Context, namespace string) error {
	return sqlitebase.RunInTx(ctx, s.db, func(ctx context.Context) error {
		if err := s.checkNamespace(ctx, namespace); err != nil {
			return err
		}
		_, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO runtime_target(id,namespace) VALUES(1,?) ON CONFLICT(id) DO NOTHING`, namespace)
		return err
	})
}

func (s *store) capture(ctx context.Context, observation domain.Observation) error {
	capture := observation.Capture()
	encoded, err := json.Marshal(capture, json.Deterministic(true))
	if err != nil {
		return err
	}
	return sqlitebase.RunInTx(ctx, s.db, func(ctx context.Context) error {
		connection := sqlitebase.Connection(ctx, s.db)
		var previous string
		err := connection.QueryRowContext(ctx, `SELECT capture FROM events WHERE generation=? AND sequence=?`, capture.Generation, capture.Sequence).Scan(&previous)
		if err == nil {
			if previous != string(encoded) {
				return errStored
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		current, err := s.configuration(ctx)
		if err != nil {
			return err
		}
		if current.Revision() != capture.Revision {
			return domain.ErrRevision
		}
		definition, err := json.Marshal(current.Snapshot().Trigger, json.Deterministic(true))
		if err != nil {
			return err
		}
		captured, err := json.Marshal(capture.Trigger, json.Deterministic(true))
		if err != nil {
			return err
		}
		if !bytes.Equal(definition, captured) {
			return errStored
		}
		var pending int
		if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE accepted=0`).Scan(&pending); err != nil {
			return err
		}
		if pending >= maxPending {
			return domain.ErrCapacity
		}
		// Only confirmed historical observations may leave the bounded read archive.
		if _, err := connection.ExecContext(ctx, `DELETE FROM events WHERE rowid IN (SELECT rowid FROM events WHERE accepted=1 ORDER BY rowid LIMIT (SELECT max(0,count(*)-?) FROM events))`, maxEvents-1); err != nil {
			return err
		}
		_, err = connection.ExecContext(ctx, `INSERT INTO events(id,generation,sequence,capture,accepted) VALUES(?,?,?,?,0)`, capture.Invocation.ID, capture.Generation, capture.Sequence, string(encoded))
		return err
	})
}

func (s *store) pending(ctx context.Context) ([]domain.Event, error) {
	result := make([]domain.Event, 0)
	err := s.visitEvents(ctx, true, func(event domain.Event) { result = append(result, event) })
	return result, err
}

func (s *store) headers(ctx context.Context) ([]domain.Header, error) {
	result := make([]domain.Header, 0)
	err := s.visitEvents(ctx, false, func(event domain.Event) { result = append(result, event.Header()) })
	return result, err
}

func (s *store) event(ctx context.Context, id string) (domain.Event, error) {
	var generation, raw string
	var sequence uint64
	var accepted int
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT generation,sequence,capture,accepted FROM events WHERE id=?`, id).Scan(&generation, &sequence, &raw, &accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Event{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Event{}, err
	}
	return decodeEvent(id, generation, sequence, raw, accepted)
}

func (s *store) visitEvents(ctx context.Context, pendingOnly bool, visit func(domain.Event)) error {
	query, limit := `SELECT id,generation,sequence,capture,accepted FROM events ORDER BY rowid DESC LIMIT ?`, maxEvents
	if pendingOnly {
		query, limit = `SELECT id,generation,sequence,capture,accepted FROM events WHERE accepted=0 ORDER BY rowid LIMIT ?`, maxPending
	}
	rows, err := sqlitebase.Connection(ctx, s.db).QueryContext(ctx, query, limit+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, generation, raw string
		var sequence uint64
		var accepted int
		if err := rows.Scan(&id, &generation, &sequence, &raw, &accepted); err != nil {
			return err
		}
		event, err := decodeEvent(id, generation, sequence, raw, accepted)
		if err != nil {
			return err
		}
		count++
		if count > limit {
			return errStored
		}
		visit(event)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

func decodeEvent(id, generation string, sequence uint64, raw string, accepted int) (domain.Event, error) {
	var capture domain.Capture
	if err := json.Unmarshal([]byte(raw), &capture, json.RejectUnknownMembers(true)); err != nil {
		return domain.Event{}, errors.Join(errStored, err)
	}
	observation, err := domain.NewObservation(capture)
	if err != nil || id != capture.Invocation.ID || generation != capture.Generation || sequence != capture.Sequence || (accepted != 0 && accepted != 1) {
		return domain.Event{}, errors.Join(errStored, err)
	}
	return domain.Event{Capture: observation.Capture(), Accepted: accepted == 1}, nil
}

func (s *store) acceptRun(ctx context.Context, id string) error {
	result, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `UPDATE events SET accepted=1 WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return errStored
	}
	return err
}
