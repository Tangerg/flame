package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/frame/sqlite"
)

type store struct{ db *sqlite.Database }

var errStoredDocument = errors.New("knowledge base: invalid stored document")

func openStore(ctx context.Context, path string) (*store, error) {
	db, err := sqlite.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		`CREATE TABLE IF NOT EXISTS documents (id TEXT PRIMARY KEY, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`,
		// Retiring an identity prevents an old revision from matching a later reimport.
		`CREATE TABLE IF NOT EXISTS retired_documents (id TEXT PRIMARY KEY)`,
	})
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return &store{db: db}, nil
}

func (s *store) read(ctx context.Context, id string) (knowledge.Document, error) {
	var raw string
	err := s.db.Read(ctx, func(reader sqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT snapshot FROM documents WHERE id=?`, id).Scan(&raw)
	})
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
	documents := make([]knowledge.Document, 0)
	err := s.db.Read(ctx, func(reader sqlite.Reader) error {
		rows, err := reader.QueryContext(ctx, `SELECT id,snapshot FROM documents ORDER BY id LIMIT ?`, knowledge.MaxDocuments+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, raw string
			if err := rows.Scan(&id, &raw); err != nil {
				return err
			}
			document, err := decodeDocument(id, raw)
			if err != nil {
				return err
			}
			documents = append(documents, document)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if len(documents) > knowledge.MaxDocuments {
		return nil, errors.New("knowledge base: stored document capacity exceeded")
	}
	return documents, nil
}

func (s *store) indexTarget(ctx context.Context, id string, expected int64) (knowledge.Document, error) {
	var current knowledge.Document
	err := sqlite.RunReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		var retired bool
		connection := sqlite.Connection(ctx, s.db)
		if err := connection.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM retired_documents WHERE id=?)`, id).Scan(&retired); err != nil {
			return err
		}
		if retired {
			return knowledge.ErrRetired
		}
		var err error
		current, err = s.read(ctx, id)
		if err != nil {
			return err
		}
		if err := current.CheckReplacement(expected, id); err != nil {
			return err
		}
		if current.Header().Revision != 0 {
			return nil
		}
		var count int
		if err := connection.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents`).Scan(&count); err != nil {
			return err
		}
		if count >= knowledge.MaxDocuments {
			return knowledge.ErrCapacity
		}
		return nil
	})
	return current, err
}

func (s *store) prepareReplacement(ctx context.Context, id string, expected int64, indexed knowledge.IndexedDraft) (pluginstorage.CommandPlan[knowledge.Header], error) {
	current, err := s.indexTarget(ctx, id, expected)
	if err != nil {
		return pluginstorage.CommandPlan[knowledge.Header]{}, err
	}
	next, err := current.Replace(expected, indexed)
	if err != nil {
		return pluginstorage.CommandPlan[knowledge.Header]{}, err
	}
	encoded, err := json.Marshal(next.Snapshot())
	if err != nil {
		return pluginstorage.CommandPlan[knowledge.Header]{}, err
	}
	return pluginstorage.NewCommandPlan(next.Header(), func(ctx context.Context) error {
		_, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO documents(id,snapshot) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET snapshot=excluded.snapshot`, id, string(encoded))
		return err
	})
}

func (s *store) prepareDeletion(ctx context.Context, id string, expected int64) (pluginstorage.CommandPlan[bool], error) {
	current, err := s.read(ctx, id)
	if err != nil {
		return pluginstorage.CommandPlan[bool]{}, err
	}
	if err := current.CheckDeletion(expected); err != nil {
		return pluginstorage.CommandPlan[bool]{}, err
	}
	return pluginstorage.NewCommandPlan(true, func(ctx context.Context) error {
		connection := sqlite.Connection(ctx, s.db)
		if _, err := connection.ExecContext(ctx, `INSERT INTO retired_documents(id) VALUES(?)`, id); err != nil {
			return err
		}
		_, err := connection.ExecContext(ctx, `DELETE FROM documents WHERE id=?`, id)
		return err
	})
}
