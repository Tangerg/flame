package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	capture "github.com/Tangerg/flame/plugins/web/backend/domain"
	sqlitebase "github.com/Tangerg/flame/packages/go-sdk/storage/sqlite"
)

type store struct{ db *sql.DB }

var errStoredSnapshot = errors.New("web: invalid stored snapshot")

func openStore(ctx context.Context, path string) (*store, error) {
	db, err := sqlitebase.OpenSchema(ctx, path, []string{`CREATE TABLE IF NOT EXISTS snapshots (id TEXT PRIMARY KEY, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`})
	if err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) read(ctx context.Context, id string) (capture.Document, error) {
	var raw string
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT snapshot FROM snapshots WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return capture.Document{}, capture.ErrNotFound
	}
	if err != nil {
		return capture.Document{}, err
	}
	var snapshot capture.Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return capture.Document{}, errors.Join(errStoredSnapshot, err)
	}
	document, err := capture.Restore(snapshot)
	if err != nil || snapshot.ID != id {
		return capture.Document{}, errors.Join(errStoredSnapshot, err)
	}
	return document, nil
}

func (s *store) capture(ctx context.Context, document capture.Document) error {
	return sqlitebase.RunInTx(ctx, s.db, func(ctx context.Context) error {
		original, err := s.read(ctx, document.Snapshot().ID)
		if err == nil {
			if original.Snapshot() != document.Snapshot() {
				return errStoredSnapshot
			}
			return nil
		}
		if !errors.Is(err, capture.ErrNotFound) {
			return err
		}
		encoded, err := json.Marshal(document.Snapshot())
		if err != nil {
			return err
		}
		if _, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO snapshots(id,snapshot) VALUES(?,?)`, document.Snapshot().ID, string(encoded)); err != nil {
			return err
		}
		_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM snapshots WHERE id IN (SELECT id FROM (SELECT id, row_number() OVER (ORDER BY rowid DESC) AS position, sum(length(CAST(snapshot AS BLOB))) OVER (ORDER BY rowid DESC) AS bytes FROM snapshots) WHERE position>? OR bytes>?)`, capture.MaxDocuments, capture.MaxArchiveBytes)
		return err
	})
}
