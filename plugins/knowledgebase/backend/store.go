package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/go/sqlite"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/backend/domain"
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

func (s *store) read(ctx context.Context, id string) (knowledge.Document, error) {
	var raw string
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT snapshot FROM documents WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return knowledge.Document{}, nil
	}
	if err != nil {
		return knowledge.Document{}, err
	}
	return decodeDocument(id, raw)
}

func decodeDocument(id, raw string) (knowledge.Document, error) {
	var snapshot knowledge.Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return knowledge.Document{}, errors.Join(errStoredDocument, err)
	}
	if snapshot.Content.ID != id {
		return knowledge.Document{}, errStoredDocument
	}
	document, err := knowledge.Restore(snapshot)
	if err != nil {
		return knowledge.Document{}, errors.Join(errStoredDocument, err)
	}
	return document, nil
}

func (s *store) documents(ctx context.Context) ([]knowledge.Document, error) {
	rows, err := sqlitebase.Connection(ctx, s.db).QueryContext(ctx, `SELECT id,snapshot FROM documents ORDER BY id LIMIT ?`, knowledge.MaxDocuments+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	documents := make([]knowledge.Document, 0)
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
	if len(documents) > knowledge.MaxDocuments {
		return nil, errors.New("knowledge base: stored document capacity exceeded")
	}
	return documents, nil
}

func (s *store) replace(ctx context.Context, id string, expected int64, indexed knowledge.IndexedDraft) (knowledge.Header, error) {
	var retired bool
	if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM retired_documents WHERE id=?)`, id).Scan(&retired); err != nil {
		return knowledge.Header{}, err
	}
	if retired {
		return knowledge.Header{}, knowledge.ErrRetired
	}
	current, err := s.read(ctx, id)
	if err != nil {
		return knowledge.Header{}, err
	}
	next, err := current.Replace(expected, indexed)
	if err != nil {
		return knowledge.Header{}, err
	}
	if current.Header().Revision == 0 {
		var count int
		if err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT COUNT(*) FROM documents`).Scan(&count); err != nil {
			return knowledge.Header{}, err
		}
		if count >= knowledge.MaxDocuments {
			return knowledge.Header{}, knowledge.ErrCapacity
		}
	}
	encoded, err := json.Marshal(next.Snapshot())
	if err != nil {
		return knowledge.Header{}, err
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

func (s *store) completeImport(ctx context.Context, prepared *pluginstorage.PreparedCommand, publish func(context.Context) (knowledge.Header, error)) (knowledge.Header, error) {
	var result knowledge.Header
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
		return knowledge.Header{}, err
	}
	return result, outcomeErr
}
