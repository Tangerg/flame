package main

import (
	"bytes"
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/sqlite"
	filewatch "github.com/Tangerg/flame/plugins/filewatch/backend/domain"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

const (
	maxPending = 32
	maxEvents  = 256
)

var errStored = errors.New("filewatch: invalid private data")

type store struct{ db *sql.DB }

func openStore(ctx context.Context, path string) (*store, error) {
	initial, err := json.Marshal(filewatch.Initial().Snapshot())
	if err != nil {
		return nil, err
	}
	db, err := sqlitebase.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		`CREATE TABLE IF NOT EXISTS configuration (id INTEGER PRIMARY KEY CHECK(id=1), snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`,
		`INSERT INTO configuration(id,snapshot) VALUES(1,'` + string(initial) + `')`,
		pluginstorage.RuntimeTargetSchema,
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

func (s *store) configuration(ctx context.Context) (filewatch.Configuration, error) {
	var raw string
	if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT snapshot FROM configuration WHERE id=1`).Scan(&raw); err != nil {
		return filewatch.Configuration{}, err
	}
	var snapshot filewatch.Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return filewatch.Configuration{}, errors.Join(errStored, err)
	}
	value, err := filewatch.Restore(snapshot)
	if err != nil {
		return filewatch.Configuration{}, errors.Join(errStored, err)
	}
	return value, nil
}

func (s *store) configure(ctx context.Context, expected int64, draft *filewatch.Draft) (filewatch.Snapshot, error) {
	current, err := s.configuration(ctx)
	if err != nil {
		return filewatch.Snapshot{}, err
	}
	next, err := current.Replace(expected, draft)
	if err != nil {
		return filewatch.Snapshot{}, err
	}
	encoded, err := json.Marshal(next.Snapshot())
	if err != nil {
		return filewatch.Snapshot{}, err
	}
	_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `UPDATE configuration SET snapshot=? WHERE id=1`, string(encoded))
	return next.Snapshot(), err
}

func (s *store) capture(ctx context.Context, observation filewatch.Observation) error {
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
			return filewatch.ErrRevision
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
			return filewatch.ErrCapacity
		}
		// Only confirmed historical observations may leave the bounded read archive.
		if _, err := connection.ExecContext(ctx, `DELETE FROM events WHERE rowid IN (SELECT rowid FROM events WHERE accepted=1 ORDER BY rowid LIMIT (SELECT max(0,count(*)-?) FROM events))`, maxEvents-1); err != nil {
			return err
		}
		_, err = connection.ExecContext(ctx, `INSERT INTO events(id,generation,sequence,capture,accepted) VALUES(?,?,?,?,0)`, capture.Invocation.ID, capture.Generation, capture.Sequence, string(encoded))
		return err
	})
}

func (s *store) pending(ctx context.Context) ([]filewatch.Event, error) {
	result := make([]filewatch.Event, 0)
	err := s.visitEvents(ctx, true, func(event filewatch.Event) { result = append(result, event) })
	return result, err
}

func (s *store) headers(ctx context.Context) ([]filewatch.Header, error) {
	result := make([]filewatch.Header, 0)
	err := s.visitEvents(ctx, false, func(event filewatch.Event) { result = append(result, event.Header()) })
	return result, err
}

func (s *store) event(ctx context.Context, id string) (filewatch.Event, error) {
	var generation, raw string
	var sequence uint64
	var accepted int
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT generation,sequence,capture,accepted FROM events WHERE id=?`, id).Scan(&generation, &sequence, &raw, &accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return filewatch.Event{}, filewatch.ErrNotFound
	}
	if err != nil {
		return filewatch.Event{}, err
	}
	return decodeEvent(id, generation, sequence, raw, accepted)
}

func (s *store) visitEvents(ctx context.Context, pendingOnly bool, visit func(filewatch.Event)) error {
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

func decodeEvent(id, generation string, sequence uint64, raw string, accepted int) (filewatch.Event, error) {
	var capture filewatch.Capture
	if err := json.Unmarshal([]byte(raw), &capture, json.RejectUnknownMembers(true)); err != nil {
		return filewatch.Event{}, errors.Join(errStored, err)
	}
	observation, err := filewatch.NewObservation(capture)
	if err != nil || id != capture.Invocation.ID || generation != capture.Generation || sequence != capture.Sequence || (accepted != 0 && accepted != 1) {
		return filewatch.Event{}, errors.Join(errStored, err)
	}
	return filewatch.Event{Capture: observation.Capture(), Accepted: accepted == 1}, nil
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
