package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/plugins/knowledgebase/domain"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	pluginstorage "github.com/Tangerg/flame/runtime/plugin/sqlite"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

type store struct{ db *sql.DB }

var errStoredDocument = errors.New("knowledge base: invalid stored document")

func openStore(ctx context.Context, path string) (*store, error) {
	db, err := sqlitebase.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		`CREATE TABLE IF NOT EXISTS documents (id TEXT PRIMARY KEY, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`,
		// Retiring an identity prevents an old revision from matching a later reimport.
		`CREATE TABLE IF NOT EXISTS retired_documents (id TEXT PRIMARY KEY)`,
	})
	if err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) read(ctx context.Context, id string) (domain.Document, error) {
	var raw string
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT snapshot FROM documents WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Document{}, nil
	}
	if err != nil {
		return domain.Document{}, err
	}
	return decodeDocument(id, raw)
}

func decodeDocument(id, raw string) (domain.Document, error) {
	var snapshot domain.Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return domain.Document{}, errors.Join(errStoredDocument, err)
	}
	if snapshot.Content.ID != id {
		return domain.Document{}, errStoredDocument
	}
	document, err := domain.Restore(snapshot)
	if err != nil {
		return domain.Document{}, errors.Join(errStoredDocument, err)
	}
	return document, nil
}

func (s *store) documents(ctx context.Context) ([]domain.Document, error) {
	rows, err := sqlitebase.Connection(ctx, s.db).QueryContext(ctx, `SELECT id,snapshot FROM documents ORDER BY id LIMIT ?`, domain.MaxDocuments+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	documents := make([]domain.Document, 0)
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		document, err := decodeDocument(id, raw)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(documents) > domain.MaxDocuments {
		return nil, errors.New("knowledge base: stored document capacity exceeded")
	}
	return documents, nil
}

func (s *store) replace(ctx context.Context, id string, expected int64, indexed domain.IndexedDraft) (domain.Header, error) {
	var retired bool
	if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM retired_documents WHERE id=?)`, id).Scan(&retired); err != nil {
		return domain.Header{}, err
	}
	if retired {
		return domain.Header{}, domain.ErrRetired
	}
	current, err := s.read(ctx, id)
	if err != nil {
		return domain.Header{}, err
	}
	next, err := current.Replace(expected, indexed)
	if err != nil {
		return domain.Header{}, err
	}
	if current.Header().Revision == 0 {
		var count int
		if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT COUNT(*) FROM documents`).Scan(&count); err != nil {
			return domain.Header{}, err
		}
		if count >= domain.MaxDocuments {
			return domain.Header{}, domain.ErrCapacity
		}
	}
	encoded, err := json.Marshal(next.Snapshot())
	if err != nil {
		return domain.Header{}, err
	}
	_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO documents(id,snapshot) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET snapshot=excluded.snapshot`, id, string(encoded))
	return next.Header(), err
}

func (s *store) delete(ctx context.Context, id string, expected int64) (bool, error) {
	current, err := s.read(ctx, id)
	if err != nil {
		return false, err
	}
	if err := current.CheckDeletion(expected); err != nil {
		return false, err
	}
	if _, err := sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO retired_documents(id) VALUES(?)`, id); err != nil {
		return false, err
	}
	_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM documents WHERE id=?`, id)
	return err == nil, err
}

func (s *store) completeImport(ctx context.Context, prepared *pluginstorage.PreparedCommand, publish func(context.Context) (domain.Header, error)) (domain.Header, error) {
	var result domain.Header
	var outcomeErr error
	err := sqlitebase.RunInTx(ctx, s.db, func(ctx context.Context) error {
		var err error
		result, outcomeErr = publish(ctx)
		reply, err := pluginsdk.Reply(result, outcomeErr, classify)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		return prepared.Complete(ctx, encoded)
	})
	if err != nil {
		return domain.Header{}, err
	}
	return result, outcomeErr
}
