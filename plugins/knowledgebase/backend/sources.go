package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/knowledge"
)

type materialSource struct {
	Header knowledge.Header `json:"header"`
	Text   string           `json:"text"`
}

func sourceOf(document knowledge.Document) materialSource {
	return materialSource{Header: document.Header(), Text: document.Content().Text}
}

func (s *store) retainSource(ctx context.Context, document knowledge.Document) error {
	source := sourceOf(document)
	if source.Header.Revision == 0 {
		return nil
	}
	value, err := json.Marshal(source)
	if err != nil {
		return err
	}
	_, err = s.db.Executor(ctx).ExecContext(ctx,
		`INSERT INTO document_history(id,revision,source) VALUES(?,?,?)`, source.Header.ID, source.Header.Revision, string(value))
	return err
}

func sourcePages(text string) []string {
	return strings.Split(strings.TrimSuffix(text, "\f"), "\f")
}

// The visible current document grants access to its retained content. A
// historical revision from a different Project does not inherit that grant.
func (s *store) resolveSource(ctx context.Context, project, id string, revision int64) (current, source materialSource, err error) {
	if _, err := identity.ParseOptionalProject(project); err != nil {
		return current, source, err
	}
	document, err := s.read(ctx, id)
	if err != nil {
		return current, source, err
	}
	if document.Header().Revision == 0 || !document.VisibleIn(project) {
		return current, source, knowledge.ErrNotFound
	}
	current = sourceOf(document)
	if revision == 0 || current.Header.Revision == revision {
		return current, current, nil
	}
	var raw string
	err = s.db.Executor(ctx).QueryRowContext(ctx,
		`SELECT source FROM document_history WHERE id=? AND revision=?`, id, revision).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return current, source, knowledge.ErrNotFound
	}
	if err != nil {
		return current, source, err
	}
	if err := json.Unmarshal([]byte(raw), &source, json.RejectUnknownMembers(true)); err != nil {
		return current, source, errors.Join(errStoredDocument, err)
	}
	if source.Header.ID != id || source.Header.Revision != revision {
		return current, source, errStoredDocument
	}
	if source.Header.Project != "" && source.Header.Project != project {
		return current, materialSource{}, knowledge.ErrNotFound
	}
	return current, source, nil
}
