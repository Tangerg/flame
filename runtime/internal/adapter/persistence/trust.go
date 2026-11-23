package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type trustStore struct{ db *database }

func newTrustStore(db *database) *trustStore {
	return &trustStore{db: db}
}

func (t *trustStore) IsTrusted(ctx context.Context, projectRoot string) (bool, error) {
	return scopedValue(ctx, t.db.read, func(ctx context.Context) (bool, error) {
		var one int
		err := t.db.connection(ctx).QueryRowContext(ctx, `SELECT 1 FROM trusted_projects WHERE project_root = ?`, projectRoot).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("sqlite: is-trusted: %w", err)
		}
		return true, nil
	})
}

func (t *trustStore) Trust(ctx context.Context, projectRoot string) error {
	return t.db.write(ctx, func(ctx context.Context) error {
		_, err := t.db.connection(ctx).ExecContext(ctx, `INSERT INTO trusted_projects (project_root) VALUES (?)
		 ON CONFLICT(project_root) DO NOTHING`, projectRoot)
		if err != nil {
			return fmt.Errorf("sqlite: trust project: %w", err)
		}
		return nil
	})
}

func (t *trustStore) Untrust(ctx context.Context, projectRoot string) error {
	return t.db.write(ctx, func(ctx context.Context) error {
		_, err := t.db.connection(ctx).ExecContext(ctx, `DELETE FROM trusted_projects WHERE project_root = ?`, projectRoot)
		if err != nil {
			return fmt.Errorf("sqlite: untrust project: %w", err)
		}
		return nil
	})
}
