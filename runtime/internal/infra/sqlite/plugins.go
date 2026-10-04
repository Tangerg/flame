package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type InstallationStore struct{ db *sql.DB }

func NewInstallationStore(db *sql.DB) *InstallationStore { return &InstallationStore{db: db} }

type installationState struct {
	Grants          []plugin.RequestGrant
	Enabled         bool
	ApprovedDigest  string
	Values          map[string]string
	DisabledServers []string
	DisabledSkills  []string
}

const installationQuery = `SELECT p.id,p.source,p.selected_digest,p.staged_digest,p.state,r.declaration,s.declaration
 FROM plugin_installations p
 JOIN plugin_releases r ON r.digest=p.selected_digest
 LEFT JOIN plugin_releases s ON s.digest=p.staged_digest`

func (s *InstallationStore) List(ctx context.Context) ([]*plugin.Installation, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx, installationQuery+` ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*plugin.Installation
	for rows.Next() {
		installation, err := scanInstallation(rows.Scan)
		if err != nil {
			return nil, err
		}
		result = append(result, installation)
	}
	return result, rows.Err()
}
func (s *InstallationStore) Get(ctx context.Context, id string) (*plugin.Installation, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx, installationQuery+` WHERE p.id=?`, id)
	installation, err := scanInstallation(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, plugin.ErrNotFound
	}
	return installation, err
}
func (s *InstallationStore) Save(ctx context.Context, installation *plugin.Installation) error {
	record := installation.Snapshot()
	state := installationState{record.Grants, record.Enabled, record.ApprovedDigest, record.Values, record.DisabledServers, record.DisabledSkills}
	body, err := encodeStoredJSON(state)
	if err != nil {
		return err
	}
	var staged any
	if record.Staged != nil {
		staged = record.Staged.Digest
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		previous, err := s.Get(ctx, record.ID)
		if err != nil && !errors.Is(err, plugin.ErrNotFound) {
			return err
		}
		_, err = conn(ctx, s.db).ExecContext(ctx, `INSERT INTO plugin_installations(id,source,selected_digest,staged_digest,state) VALUES(?,?,?,?,?)
   ON CONFLICT(id) DO UPDATE SET source=excluded.source,selected_digest=excluded.selected_digest,staged_digest=excluded.staged_digest,state=excluded.state`, record.ID, record.Source, record.Selected.Digest, staged, string(body))
		if err != nil {
			return err
		}
		// Validate the hydrated aggregate before commit: a caller cannot smuggle a
		// new declaration through an installation's read projection.
		current, err := s.Get(ctx, record.ID)
		if err != nil {
			return err
		}
		sources, err := installationSources(current.Snapshot())
		if err != nil {
			return err
		}
		if previous != nil {
			oldSources, err := installationSources(previous.Snapshot())
			if err != nil {
				return err
			}
			for _, source := range oldSources {
				if !slices.Contains(sources, source) {
					if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM mcp_sources WHERE name=?`, source); err != nil {
						return err
					}
					continue
				}
				name, err := mcpserver.ParseServerName(source)
				if err != nil {
					return err
				}
				if !current.RetainsServerCredentials(previous, name.Local()) {
					if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM mcp_oauth_sessions WHERE server_name=?`, source); err != nil {
						return err
					}
				}
			}
		}
		for _, source := range sources {
			if _, err := conn(ctx, s.db).ExecContext(ctx, `INSERT INTO mcp_sources(name,installation_id) VALUES(?,?) ON CONFLICT(name) DO NOTHING`, source, record.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *InstallationStore) Remove(ctx context.Context, id string) error {
	result, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM plugin_installations WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return plugin.ErrNotFound
	}
	return nil
}
func installationSources(record plugin.Record) ([]string, error) {
	result := make([]string, 0, len(record.Selected.Servers))
	for _, server := range record.Selected.Servers {
		name, err := mcpserver.InstallationServer(record.ID, server.Name)
		if err != nil {
			return nil, err
		}
		result = append(result, name.String())
	}
	return result, nil
}
func scanInstallation(scan func(...any) error) (*plugin.Installation, error) {
	var record plugin.Record
	var selected, body, declaration string
	var staged, stagedDeclaration sql.NullString
	if err := scan(&record.ID, &record.Source, &selected, &staged, &body, &declaration, &stagedDeclaration); err != nil {
		return nil, err
	}
	release, err := decodeRelease(selected, declaration)
	if err != nil {
		return nil, err
	}
	record.Selected = release
	if staged.Valid {
		release, err := decodeRelease(staged.String, stagedDeclaration.String)
		if err != nil {
			return nil, err
		}
		record.Staged = &release
	}
	var state installationState
	if err := decodeStoredJSON([]byte(body), &state); err != nil {
		return nil, fmt.Errorf("sqlite: decode installation state: %w", err)
	}
	record.Grants, record.Enabled, record.ApprovedDigest = state.Grants, state.Enabled, state.ApprovedDigest
	record.Values, record.DisabledServers, record.DisabledSkills = state.Values, state.DisabledServers, state.DisabledSkills
	return plugin.Restore(record)
}
