package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	framevalue "github.com/Tangerg/frame/value"
)

type ReleaseStore struct {
	db          *database
	mu          sync.Mutex
	projections map[framevalue.Digest]releaseProjection
}
type releaseProjection struct {
	declaration string
	release     plugin.Release
}

func newReleaseStore(db *database) *ReleaseStore {
	return &ReleaseStore{db: db, projections: make(map[framevalue.Digest]releaseProjection)}
}

func (s *ReleaseStore) Get(ctx context.Context, digest framevalue.Digest) (plugin.Release, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) (plugin.Release, error) {
		var body string
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT declaration FROM plugin_releases WHERE digest=?`, digest.String()).Scan(&body)
		if errors.Is(err, sql.ErrNoRows) {
			return plugin.Release{}, plugin.ErrNotFound
		}
		if err != nil {
			return plugin.Release{}, err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if projection, ok := s.projections[digest]; ok && projection.declaration == body {
			return projection.release, nil
		}
		var stored storedDeclaration
		if err := decodeStoredJSON([]byte(body), &stored); err != nil {
			return plugin.Release{}, fmt.Errorf("sqlite: decode release %s: %w", digest, err)
		}
		declaration, err := stored.declaration()
		if err != nil {
			return plugin.Release{}, fmt.Errorf("sqlite: decode release %s: %w", digest, err)
		}
		release, err := plugin.NewRelease(digest, declaration)
		if err != nil {
			return plugin.Release{}, err
		}
		s.projections[digest] = releaseProjection{declaration: body, release: release}
		return release, nil
	})
}

func (s *ReleaseStore) Admit(ctx context.Context, release plugin.Release) error {
	declaration := release.Declaration()
	body, err := encodeStoredJSON(storeDeclaration(declaration))
	if err != nil {
		return err
	}
	digest := release.Digest().String()
	return s.db.write(ctx, func(ctx context.Context) error {
		db := s.db.connection(ctx)
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

func (s *ReleaseStore) Digests(ctx context.Context) ([]framevalue.Digest, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) ([]framevalue.Digest, error) {
		rows, err := s.db.connection(ctx).QueryContext(ctx, `SELECT digest FROM plugin_releases ORDER BY digest`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var result []framevalue.Digest
		for rows.Next() {
			var text string
			if err := rows.Scan(&text); err != nil {
				return nil, err
			}
			digest, err := framevalue.ParseDigest(text)
			if err != nil {
				return nil, fmt.Errorf("sqlite: decode release digest: %w", err)
			}
			result = append(result, digest)
		}
		return result, rows.Err()
	})
}

func (s *ReleaseStore) Remove(ctx context.Context, digest framevalue.Digest) error {
	return s.db.write(ctx, func(ctx context.Context) error {
		_, err := s.db.connection(ctx).ExecContext(ctx, `DELETE FROM plugin_releases WHERE digest=?`, digest.String())
		if err != nil {
			return err
		}
		s.mu.Lock()
		delete(s.projections, digest)
		s.mu.Unlock()
		return nil
	})
}
