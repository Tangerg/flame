package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// ReleaseStore is an insert-only declaration projection of admitted package
// bytes. Installations reference a digest and cannot replace its declaration.
type ReleaseStore struct{ db *sql.DB }

func NewReleaseStore(db *sql.DB) *ReleaseStore { return &ReleaseStore{db: db} }

func (s *ReleaseStore) Get(ctx context.Context, digest string) (plugin.Release, error) {
	var body string
	err := conn(ctx, s.db).QueryRowContext(ctx, `SELECT declaration FROM plugin_releases WHERE digest=?`, digest).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return plugin.Release{}, plugin.ErrNotFound
	}
	if err != nil {
		return plugin.Release{}, err
	}
	return decodeRelease(digest, body)
}

func (s *ReleaseStore) Admit(ctx context.Context, release plugin.Release) error {
	if err := release.Validate(); err != nil {
		return err
	}
	digest := release.Digest
	release.Digest = ""
	body, err := encodeStoredJSON(release)
	if err != nil {
		return err
	}
	_, err = conn(ctx, s.db).ExecContext(ctx, `INSERT INTO plugin_releases(digest,declaration) VALUES(?,?) ON CONFLICT(digest) DO NOTHING`, digest, string(body))
	return err
}

func decodeRelease(digest, body string) (plugin.Release, error) {
	var release plugin.Release
	if err := decodeStoredJSON([]byte(body), &release); err != nil {
		return release, fmt.Errorf("sqlite: decode release: %w", err)
	}
	if release.Digest != "" {
		return release, errors.New("sqlite: release declaration contains a digest")
	}
	release.Digest = digest
	return release, release.Validate()
}
