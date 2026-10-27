package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/qimu/sqlite"
)

// TrustStore records which project roots are trusted to run their project-scope
// hooks (internal/domain/integration/hooks). A cloned repo's hooks stay inert until the user
// trusts the project explicitly. Global (~/.flame) hooks need no entry.
type trustStore struct {
	db *sql.DB
}

// NewTrustStore wires the given *sql.DB to the trusted-projects table.
func newTrustStore(db *sql.DB) *trustStore {
	return &trustStore{db: db}
}

// IsTrusted reports whether projectRoot has been granted hook trust.
func (t *trustStore) IsTrusted(ctx context.Context, projectRoot string) (bool, error) {
	var one int
	err := sqlite.Connection(ctx, t.db).QueryRowContext(ctx,
		`SELECT 1 FROM trusted_projects WHERE project_root = ?`, projectRoot).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: is-trusted: %w", err)
	}
	return true, nil
}

// Trust grants hook trust to projectRoot. Repeated grants are idempotent.
func (t *trustStore) Trust(ctx context.Context, projectRoot string) error {
	_, err := sqlite.Connection(ctx, t.db).ExecContext(ctx,
		`INSERT INTO trusted_projects (project_root) VALUES (?)
		 ON CONFLICT(project_root) DO NOTHING`,
		projectRoot)
	if err != nil {
		return fmt.Errorf("sqlite: trust project: %w", err)
	}
	return nil
}

// Untrust revokes hook trust for projectRoot. Idempotent.
func (t *trustStore) Untrust(ctx context.Context, projectRoot string) error {
	_, err := sqlite.Connection(ctx, t.db).ExecContext(ctx,
		`DELETE FROM trusted_projects WHERE project_root = ?`, projectRoot)
	if err != nil {
		return fmt.Errorf("sqlite: untrust project: %w", err)
	}
	return nil
}
