package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/agent/projects"
	"github.com/Tangerg/flame/runtime/internal/domain/project"
)

const projectColumns = `id, name, instructions, archived, created_at, updated_at, revision`

type ProjectStore struct {
	db *database
}

var _ projects.Store = (*ProjectStore)(nil)

func (s *ProjectStore) Insert(ctx context.Context, value project.Project) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		snapshot := value.Snapshot()
		if snapshot.Revision != exactint.First().Value() {
			return fmt.Errorf("sqlite: initial Project revision is %d: %w", snapshot.Revision, project.ErrInvalid)
		}
		_, err := s.db.connection(ctx).ExecContext(ctx, `INSERT INTO projects(`+projectColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			snapshot.ID, snapshot.Name, snapshot.Instructions, boolToInt(snapshot.Archived),
			snapshot.CreatedAt.UnixNano(), snapshot.UpdatedAt.UnixNano(), snapshot.Revision)
		if err != nil {
			return fmt.Errorf("sqlite: insert Project: %w", err)
		}
		return nil
	})
}

func (s *ProjectStore) Save(ctx context.Context, expectedRevision uint64, value project.Project) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		snapshot := value.Snapshot()
		result, err := s.db.connection(ctx).ExecContext(ctx, `UPDATE projects SET
		name = ?, instructions = ?, archived = ?, updated_at = ?, revision = ?
		WHERE id = ? AND revision = ?`, snapshot.Name, snapshot.Instructions, boolToInt(snapshot.Archived),
			snapshot.UpdatedAt.UnixNano(), snapshot.Revision, snapshot.ID, expectedRevision)
		if err != nil {
			return fmt.Errorf("sqlite: save Project: %w", err)
		}
		written, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect Project save: %w", err)
		}
		if written == 1 {
			return nil
		}
		if _, err := s.Get(ctx, snapshot.ID); err != nil {
			return err
		}
		return project.ErrRevisionConflict
	})
}

func (s *ProjectStore) Get(ctx context.Context, id identity.ProjectID) (project.Project, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) (project.Project, error) {
		if err := validateResource("read Project", id); err != nil {
			return project.Project{}, err
		}
		value, err := rowToProject(s.db.connection(ctx).QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE id = ?`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return project.Project{}, project.ErrNotFound
		}
		if err != nil {
			return project.Project{}, fmt.Errorf("sqlite: get Project: %w", err)
		}
		return value, nil
	})
}

func (s *ProjectStore) ListPage(ctx context.Context, read projects.CatalogRead) ([]project.Project, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) ([]project.Project, error) {
		query := `SELECT ` + projectColumns + ` FROM projects WHERE archived = ?`
		args := []any{boolToInt(read.Archived)}
		if read.After != nil {
			updatedAt := read.After.UpdatedAt.UnixNano()
			query += ` AND (updated_at < ? OR (updated_at = ? AND id < ?))`
			args = append(args, updatedAt, updatedAt, read.After.ID)
		}
		query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
		args = append(args, read.Limit)
		rows, err := s.db.connection(ctx).QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list Projects: %w", err)
		}
		defer rows.Close()
		var out []project.Project
		for rows.Next() {
			value, err := rowToProject(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: list Projects: %w", err)
		}
		return out, nil
	})
}

func rowToProject(scanner interface{ Scan(dest ...any) error }) (project.Project, error) {
	var (
		snapshot       project.Snapshot
		archived       int64
		createdAtNanos int64
		updatedAtNanos int64
	)
	if err := scanner.Scan(&snapshot.ID, &snapshot.Name, &snapshot.Instructions, &archived,
		&createdAtNanos, &updatedAtNanos, &snapshot.Revision); err != nil {
		return project.Project{}, err
	}
	var err error
	if snapshot.Archived, err = boolFromInt(archived); err != nil {
		return project.Project{}, fmt.Errorf("sqlite: Project archive state: %w", err)
	}
	snapshot.CreatedAt = time.Unix(0, createdAtNanos).UTC()
	snapshot.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	return project.Restore(snapshot)
}
