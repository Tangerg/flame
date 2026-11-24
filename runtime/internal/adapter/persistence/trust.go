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

func (t *trustStore) IsTrusted(ctx context.Context, repositoryRoot string) (bool, error) {
	return scopedValue(ctx, t.db.read, func(ctx context.Context) (bool, error) {
		var one int
		err := t.db.connection(ctx).QueryRowContext(ctx, `SELECT 1 FROM trusted_repositories WHERE repository_root = ?`, repositoryRoot).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("sqlite: is-trusted: %w", err)
		}
		return true, nil
	})
}

func (t *trustStore) Trust(ctx context.Context, repositoryRoot string) error {
	return t.db.write(ctx, func(ctx context.Context) error {
		_, err := t.db.connection(ctx).ExecContext(ctx, `INSERT INTO trusted_repositories (repository_root) VALUES (?)
		 ON CONFLICT(repository_root) DO NOTHING`, repositoryRoot)
		if err != nil {
			return fmt.Errorf("sqlite: trust repository: %w", err)
		}
		return nil
	})
}

func (t *trustStore) Untrust(ctx context.Context, repositoryRoot string) error {
	return t.db.write(ctx, func(ctx context.Context) error {
		_, err := t.db.connection(ctx).ExecContext(ctx, `DELETE FROM trusted_repositories WHERE repository_root = ?`, repositoryRoot)
		if err != nil {
			return fmt.Errorf("sqlite: untrust repository: %w", err)
		}
		return nil
	})
}
