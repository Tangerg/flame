package sqlite

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	framesqlite "github.com/Tangerg/frame/sqlite"
)

var ErrStoredSnapshot = errors.New("managed plugin: invalid stored snapshot")

type Snapshot[S any] struct {
	db    framesqlite.Database
	table string
}

func SnapshotSchema(table string) string {
	return `CREATE TABLE IF NOT EXISTS ` + table + ` (id INTEGER PRIMARY KEY CHECK(id=1), snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`
}

func OpenSnapshot[S any](ctx context.Context, db framesqlite.Database, table string, initial S) (*Snapshot[S], error) {
	encoded, err := json.Marshal(initial, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("managed plugin: encode initial %s: %w", table, err)
	}
	snapshot := &Snapshot[S]{db: db, table: table}
	err = db.RunInTx(ctx, func(ctx context.Context) error {
		_, err := db.Executor(ctx).ExecContext(ctx, `INSERT INTO `+table+`(id,snapshot) VALUES(1,?) ON CONFLICT(id) DO NOTHING`, string(encoded))
		return err
	})
	if err != nil {
		return nil, err
	}
	if _, err := snapshot.Read(ctx); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *Snapshot[S]) Read(ctx context.Context) (S, error) {
	var encoded string
	var value S
	if err := s.db.Read(ctx, func(reader framesqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT snapshot FROM `+s.table+` WHERE id=1`).Scan(&encoded)
	}); err != nil {
		return value, errors.Join(ErrStoredSnapshot, err)
	}
	if err := json.Unmarshal([]byte(encoded), &value, json.RejectUnknownMembers(true)); err != nil {
		return value, errors.Join(ErrStoredSnapshot, err)
	}
	return value, nil
}

func Replace[S, R any](s *Snapshot[S], result R, next S) (CommandPlan[R], error) {
	encoded, err := json.Marshal(next, json.Deterministic(true))
	if err != nil {
		return CommandPlan[R]{}, fmt.Errorf("managed plugin: encode %s: %w", s.table, err)
	}
	return NewCommandPlan(result, func(ctx context.Context) error {
		_, err := s.db.Executor(ctx).ExecContext(ctx, `UPDATE `+s.table+` SET snapshot=? WHERE id=1`, string(encoded))
		return err
	})
}
