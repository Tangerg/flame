package sqlite

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	framesqlite "github.com/Tangerg/frame/sqlite"
)

// ErrStoredSnapshot reports a stored snapshot its owner can no longer decode.
var ErrStoredSnapshot = errors.New("managed plugin: invalid stored snapshot")

// Snapshot is a plugin's single durable JSON value, such as a revisioned
// policy. Its domain value owns the revision and validation; the row stores
// only the snapshot that restores it.
type Snapshot[S any] struct {
	db    framesqlite.Database
	table string
}

// SnapshotSchema declares the single-row table of a Snapshot.
func SnapshotSchema(table string) string {
	return `CREATE TABLE IF NOT EXISTS ` + table + ` (id INTEGER PRIMARY KEY CHECK(id=1), snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`
}

// OpenSnapshot seeds an empty table with initial and keeps any stored value,
// which must still decode.
func OpenSnapshot[S any](ctx context.Context, db framesqlite.Database, table string, initial S) (*Snapshot[S], error) {
	encoded, err := json.Marshal(initial, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("managed plugin: encode initial %s: %w", table, err)
	}
	snapshot := &Snapshot[S]{db: db, table: table}
	err = framesqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		_, err := framesqlite.Connection(ctx, db).ExecContext(ctx, `INSERT INTO `+table+`(id,snapshot) VALUES(1,?) ON CONFLICT(id) DO NOTHING`, string(encoded))
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

// Read decodes the stored value, rejecting members its type no longer declares.
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

// Replace plans storing next as the command's durable effect.
func Replace[S, R any](s *Snapshot[S], result R, next S) (CommandPlan[R], error) {
	encoded, err := json.Marshal(next, json.Deterministic(true))
	if err != nil {
		return CommandPlan[R]{}, fmt.Errorf("managed plugin: encode %s: %w", s.table, err)
	}
	return NewCommandPlan(result, func(ctx context.Context) error {
		_, err := framesqlite.Connection(ctx, s.db).ExecContext(ctx, `UPDATE `+s.table+` SET snapshot=? WHERE id=1`, string(encoded))
		return err
	})
}
