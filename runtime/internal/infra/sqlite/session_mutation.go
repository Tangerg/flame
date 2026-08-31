package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/exactint"
)

// Insert persists one already-decided initial Session. Identity, timestamps,
// lineage, editable values, and revision all belong to the aggregate.
func (s *SessionStore) Insert(ctx context.Context, value session.Session) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: validate initial Session: %w", err)
	}
	firstRevision := exactint.First().Value()
	if value.Revision() != firstRevision {
		return fmt.Errorf("sqlite: initial Session revision is %d, want %d: %w", value.Revision(), firstRevision, session.ErrInvalid)
	}
	if err := s.execInsert(ctx, conn(ctx, s.db), value); err != nil {
		return err
	}
	return nil
}

// Save persists an application-decided replacement iff expectedRevision is
// still current. It never normalizes fields, reads a clock, applies a Patch, or
// assigns the next revision.
func (s *SessionStore) Save(
	ctx context.Context,
	expectedRevision uint64,
	replacement session.Session,
) error {
	if err := replacement.Validate(); err != nil {
		return fmt.Errorf("sqlite: validate Session replacement: %w", err)
	}
	if expectedRevision == 0 || exactint.Follows(expectedRevision, replacement.Revision()) != nil {
		return fmt.Errorf(
			"sqlite: Session replacement revision %d does not follow expected revision %d: %w",
			replacement.Revision(), expectedRevision, session.ErrInvalid,
		)
	}
	snapshot := replacement.Snapshot()
	titleSearch, err := session.NormalizeCatalogText(snapshot.Title)
	if err != nil {
		return fmt.Errorf("sqlite: normalize Session title search material: %w", err)
	}
	workspaceSearch, err := session.NormalizeCatalogText(snapshot.Workspace.Path())
	if err != nil {
		return fmt.Errorf("sqlite: normalize Session workspace search material: %w", err)
	}
	result, err := conn(ctx, s.db).ExecContext(ctx, `UPDATE sessions SET
		title = ?, title_search = ?, workspace_path = ?, workspace_search = ?, parent_id = ?, started_at = ?, updated_at = ?,
		provider = ?, model = ?, reasoning_effort = ?, favorite = ?, isolated = ?, revision = ?
		WHERE id = ? AND revision = ?`,
		snapshot.Title, titleSearch, snapshot.Workspace.Path(), workspaceSearch, snapshot.ParentID,
		snapshot.StartedAt.UnixNano(), snapshot.UpdatedAt.UnixNano(),
		snapshot.Selection.Provider(), snapshot.Selection.Model(), snapshot.Selection.ReasoningEffort(),
		boolToInt(snapshot.Favorite), boolToInt(snapshot.Isolated),
		snapshot.Revision, snapshot.ID, expectedRevision,
	)
	if err != nil {
		return fmt.Errorf("sqlite: save Session: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect Session save: %w", err)
	}
	if written == 1 {
		return nil
	}
	if _, err := s.Get(ctx, snapshot.ID); errors.Is(err, session.ErrNotFound) {
		return session.ErrNotFound
	} else if err != nil {
		return err
	}
	return session.ErrRevisionConflict
}

// Delete is idempotent. It joins an ambient transaction through conn(ctx).
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	if err := validateSessionResource("delete Session", id); err != nil {
		return err
	}
	if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("sqlite: delete Session: %w", err)
	}
	return nil
}

func (s *SessionStore) execInsert(ctx context.Context, executor execer, value session.Session) error {
	snapshot := value.Snapshot()
	titleSearch, err := session.NormalizeCatalogText(snapshot.Title)
	if err != nil {
		return fmt.Errorf("sqlite: normalize Session title search material: %w", err)
	}
	workspaceSearch, err := session.NormalizeCatalogText(snapshot.Workspace.Path())
	if err != nil {
		return fmt.Errorf("sqlite: normalize Session workspace search material: %w", err)
	}
	_, err = executor.ExecContext(ctx,
		`INSERT INTO sessions(`+sessionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.ID, snapshot.Title, titleSearch, snapshot.Workspace.Path(), workspaceSearch, snapshot.ParentID,
		snapshot.StartedAt.UnixNano(), snapshot.UpdatedAt.UnixNano(),
		snapshot.Selection.Provider(), snapshot.Selection.Model(), snapshot.Selection.ReasoningEffort(),
		boolToInt(snapshot.Favorite), boolToInt(snapshot.Isolated),
		snapshot.Revision,
	)
	if err != nil {
		return fmt.Errorf("sqlite: insert Session: %w", err)
	}
	return nil
}
