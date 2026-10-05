package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// ReleaseStore is the admitted release catalog: the only owner of release
// content. Rows are insert-only by digest, so an installation names a release
// and can never replace its declaration.
type ReleaseStore struct{ db *sql.DB }

func NewReleaseStore(db *sql.DB) *ReleaseStore { return &ReleaseStore{db: db} }

func (s *ReleaseStore) Get(ctx context.Context, digest fingerprint.Digest) (plugin.Release, error) {
	var body string
	err := conn(ctx, s.db).QueryRowContext(ctx, `SELECT declaration FROM plugin_releases WHERE digest=?`, digest.String()).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return plugin.Release{}, plugin.ErrNotFound
	}
	if err != nil {
		return plugin.Release{}, err
	}
	var declaration plugin.Declaration
	if err := decodeStoredJSON([]byte(body), &declaration); err != nil {
		return plugin.Release{}, fmt.Errorf("sqlite: decode release %s: %w", digest, err)
	}
	return plugin.NewRelease(digest, declaration)
}

// Admit records a release and, in the same transaction, the names of the
// servers it declares. Membership is written once with the release and never
// changes, so installation sources follow it relationally.
func (s *ReleaseStore) Admit(ctx context.Context, release plugin.Release) error {
	declaration := release.Declaration()
	body, err := encodeStoredJSON(declaration)
	if err != nil {
		return err
	}
	digest := release.Digest().String()
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		db := conn(ctx, s.db)
		result, err := db.ExecContext(ctx, `INSERT INTO plugin_releases(digest,declaration) VALUES(?,?) ON CONFLICT(digest) DO NOTHING`, digest, string(body))
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil || inserted == 0 {
			return err
		}
		for _, server := range declaration.Servers {
			if _, err := db.ExecContext(ctx, `INSERT INTO plugin_release_servers(digest,name) VALUES(?,?)`, digest, server.Name.String()); err != nil {
				return err
			}
		}
		return nil
	})
}

// Digests lists every admitted release.
func (s *ReleaseStore) Digests(ctx context.Context) ([]fingerprint.Digest, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx, `SELECT digest FROM plugin_releases ORDER BY digest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []fingerprint.Digest
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		digest, err := fingerprint.ParseDigest(text)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode release digest: %w", err)
		}
		result = append(result, digest)
	}
	return result, rows.Err()
}

// Remove deletes an unreferenced release with its server membership. An
// installation still naming it makes the delete fail, so a referenced release
// cannot be removed whatever its caller believes.
func (s *ReleaseStore) Remove(ctx context.Context, digest fingerprint.Digest) error {
	_, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM plugin_releases WHERE digest=?`, digest.String())
	return err
}
