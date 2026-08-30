package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// List returns user-facing sessions (roots and forks), newest-updated first.
func (s *SessionStore) List(ctx context.Context) ([]session.Session, error) {
	return s.list(ctx, session.AllCatalogEntries(), nil, 0)
}

// ListPage returns user-facing sessions in list order, bounded by the query. The
// anchor is the full sort key a previous page ended at — pinned state, then
// update time, then id — because the earlier components tie freely and a partial
// bound would drop or repeat rows at a page boundary.
//
// Paging here rather than after the fact matters more than for most reads: each
// session's view is resolved against the filesystem and the live-run registry, so
// slicing a fully-resolved list did that work for every session to return one
// page of them.
func (s *SessionStore) ListPage(ctx context.Context, read session.CatalogRead) ([]session.Session, error) {
	if err := read.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: list sessions: %w", err)
	}
	var after *session.CatalogAnchor
	if value, present := read.After(); present {
		after = &value
	}
	return s.list(ctx, read.Filter(), after, read.Limit())
}

func (s *SessionStore) list(
	ctx context.Context,
	filter session.CatalogFilter,
	after *session.CatalogAnchor,
	limit int,
) ([]session.Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions`
	var predicates []string
	var args []any
	if search, present := filter.Search(); present {
		pattern := "%" + escapeSessionLikePattern(search) + "%"
		predicates = append(predicates, `(title_search LIKE ? ESCAPE '\' OR workspace_search LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern)
	}
	if workspacePath, present := filter.WorkspacePath(); present {
		predicates = append(predicates, `workspace_path = ?`)
		args = append(args, workspacePath)
	}
	if after != nil {
		favorite := 0
		if after.Favorite() {
			favorite = 1
		}
		updatedAt := after.UpdatedAt().UnixNano()
		predicates = append(predicates, `(favorite < ?
			OR (favorite = ? AND updated_at < ?)
			OR (favorite = ? AND updated_at = ? AND id < ?))`)
		args = append(args, favorite, favorite, updatedAt, favorite, updatedAt, after.ID())
	}
	if len(predicates) > 0 {
		query += ` WHERE ` + strings.Join(predicates, ` AND `)
	}
	query += ` ORDER BY favorite DESC, updated_at DESC, id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]session.Session, 0)
	for rows.Next() {
		sess, err := rowToSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list sessions: %w", err)
	}
	return out, nil
}

func escapeSessionLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// Exists reports whether a session row exists — the cheap existence check the
// goal driver uses to refuse a goal for a missing session and to sweep orphaned
// goals at boot, without decoding the whole aggregate.
func (s *SessionStore) Exists(ctx context.Context, id string) (bool, error) {
	if err := validateSessionResource("check Session existence", id); err != nil {
		return false, err
	}
	var one int
	err := conn(ctx, s.db).QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: session exists: %w", err)
	}
	return true, nil
}

// ModelSelection returns the Session's exact durable model policy without
// decoding unrelated aggregate state. Goal admission uses the boolean as its
// existence fact and freezes the returned selection when no override was sent.
func (s *SessionStore) ModelSelection(ctx context.Context, id string) (modelref.Selection, bool, error) {
	if err := validateSessionResource("read Session model selection", id); err != nil {
		return modelref.Selection{}, false, err
	}
	var provider, model, reasoningEffort string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT provider, model, reasoning_effort FROM sessions WHERE id = ?`, id,
	).Scan(&provider, &model, &reasoningEffort)
	if errors.Is(err, sql.ErrNoRows) {
		return modelref.Selection{}, false, nil
	}
	if err != nil {
		return modelref.Selection{}, false, fmt.Errorf("sqlite: read Session model selection: %w", err)
	}
	selection, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return modelref.Selection{}, false, fmt.Errorf("sqlite: decode Session model selection: %w", err)
	}
	return selection, true, nil
}

func (s *SessionStore) Get(ctx context.Context, id string) (session.Session, error) {
	if err := validateSessionResource("read Session", id); err != nil {
		return session.Session{}, err
	}
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	sess, err := rowToSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Session{}, session.ErrNotFound
	}
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: get session: %w", err)
	}
	return sess, nil
}
