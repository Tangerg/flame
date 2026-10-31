package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/qimu/integrity"
	"github.com/Tangerg/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// ReleaseStore is the admitted release catalog: the only owner of release
// content. Rows are insert-only by digest, so an installation names a release
// and can never replace its declaration.
type ReleaseStore struct {
	db          *sql.DB
	mu          sync.Mutex
	projections map[integrity.Digest]releaseProjection
}

type releaseProjection struct {
	declaration string
	release     plugin.Release
}

func newReleaseStore(db *sql.DB) *ReleaseStore {
	return &ReleaseStore{db: db, projections: make(map[integrity.Digest]releaseProjection)}
}

func (s *ReleaseStore) Get(ctx context.Context, digest integrity.Digest) (plugin.Release, error) {
	var body string
	err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT declaration FROM plugin_releases WHERE digest=?`, digest.String()).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return plugin.Release{}, plugin.ErrNotFound
	}
	if err != nil {
		return plugin.Release{}, err
	}
	// The current SQL read owns existence and transaction visibility. Only
	// admission of its complete declaration may be reused across snapshots.
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
}

// Admit records a release and, in the same transaction, the names of the
// servers it declares. Membership is written once with the release and never
// changes, so installation sources follow it relationally.
func (s *ReleaseStore) Admit(ctx context.Context, release plugin.Release) error {
	declaration := release.Declaration()
	body, err := encodeStoredJSON(storeDeclaration(declaration))
	if err != nil {
		return err
	}
	digest := release.Digest().String()
	return sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		db := sqlite.Connection(ctx, s.db)
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
func (s *ReleaseStore) Digests(ctx context.Context) ([]integrity.Digest, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT digest FROM plugin_releases ORDER BY digest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []integrity.Digest
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		digest, err := integrity.ParseDigest(text)
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
func (s *ReleaseStore) Remove(ctx context.Context, digest integrity.Digest) error {
	_, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `DELETE FROM plugin_releases WHERE digest=?`, digest.String())
	if err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.projections, digest)
	s.mu.Unlock()
	return nil
}

// storedDeclaration is the release record. Its servers are stored flat; the
// shadowing field replaces the domain's startup variants in the encoding.
type storedDeclaration struct {
	plugin.Declaration
	Servers []storedServer
}

type storedServer struct {
	Name      mcpserver.ServerName
	Managed   *plugin.ManagedBackend `json:",omitzero"`
	Transport mcpserver.Transport
	Command   string
	Args      []string
	Env       map[string]string
	Dir       string
	URL       string
	Headers   map[string]string
}

func storeDeclaration(declaration plugin.Declaration) storedDeclaration {
	stored := storedDeclaration{Declaration: declaration, Servers: make([]storedServer, len(declaration.Servers))}
	stored.Declaration.Servers = nil
	for index, server := range declaration.Servers {
		record := storedServer{Name: server.Name, Transport: server.Transport()}
		switch startup := server.Startup.(type) {
		case plugin.ManagedBackend:
			record.Managed = &startup
		case plugin.DeclaredConnection:
			switch connection := startup.Connection.(type) {
			case mcpserver.StdioConnection:
				record.Command, record.Args, record.Env, record.Dir = connection.Command, connection.Args, connection.Env, connection.Dir
			case mcpserver.HTTPConnection:
				record.URL, record.Headers = connection.URL, connection.Headers
			}
		}
		stored.Servers[index] = record
	}
	return stored
}

func (s storedDeclaration) declaration() (plugin.Declaration, error) {
	declaration := s.Declaration
	declaration.Servers = make([]plugin.Server, len(s.Servers))
	for index, record := range s.Servers {
		server := plugin.Server{Name: record.Name}
		switch {
		case record.Managed != nil:
			server.Startup = *record.Managed
		case record.Transport == mcpserver.TransportStdio:
			server.Startup = plugin.DeclaredConnection{Connection: mcpserver.StdioConnection{Command: record.Command, Args: record.Args, Env: record.Env, Dir: record.Dir}}
		case record.Transport == mcpserver.TransportStreamableHTTP:
			server.Startup = plugin.DeclaredConnection{Connection: mcpserver.HTTPConnection{URL: record.URL, Headers: record.Headers}}
		default:
			return plugin.Declaration{}, fmt.Errorf("server %q has unknown transport %q", record.Name, record.Transport)
		}
		declaration.Servers[index] = server
	}
	return declaration, nil
}
