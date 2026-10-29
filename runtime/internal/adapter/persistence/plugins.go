package persistence

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type InstallationStore struct {
	db       *sql.DB
	releases *ReleaseStore
}

func newInstallationStore(releases *ReleaseStore) *InstallationStore {
	return &InstallationStore{db: releases.db, releases: releases}
}

const (
	disabledServer = "server"
	disabledSkill  = "skill"
)

func (s *InstallationStore) List(ctx context.Context) ([]plugin.Snapshot, error) {
	var result []plugin.Snapshot
	err := sqlite.RunReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		records, err := s.records(ctx, `SELECT id,source,selected_digest,staged_digest,enabled FROM plugin_installations ORDER BY id`)
		if err != nil {
			return err
		}
		for _, record := range records {
			installation, err := s.restore(ctx, record)
			if err != nil {
				return err
			}
			result = append(result, installation)
		}
		return nil
	})
	return result, err
}

func (s *InstallationStore) Get(ctx context.Context, id identity.InstallationID) (plugin.Snapshot, bool, error) {
	var result plugin.Snapshot
	err := sqlite.RunReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		records, err := s.records(ctx, `SELECT id,source,selected_digest,staged_digest,enabled FROM plugin_installations WHERE id=?`, id.String())
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		result, err = s.restore(ctx, records[0])
		return err
	})
	if err != nil {
		return plugin.Snapshot{}, false, err
	}
	return result, result.Installation != nil, nil
}

func (s *InstallationStore) records(ctx context.Context, query string, args ...any) ([]plugin.Record, error) {
	rows, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []plugin.Record
	for rows.Next() {
		var record plugin.Record
		var id, selected string
		var staged sql.NullString
		if err := rows.Scan(&id, &record.Source, &selected, &staged, &record.Enabled); err != nil {
			return nil, err
		}
		if record.ID, err = identity.ParseInstallation(id); err != nil {
			return nil, fmt.Errorf("sqlite: decode installation identity: %w", err)
		}
		if record.Selected, err = identity.ParseDigest(selected); err != nil {
			return nil, fmt.Errorf("sqlite: decode selected release: %w", err)
		}
		if staged.Valid {
			digest, err := identity.ParseDigest(staged.String)
			if err != nil {
				return nil, fmt.Errorf("sqlite: decode staged release: %w", err)
			}
			record.Staged = &digest
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *InstallationStore) restore(ctx context.Context, record plugin.Record) (plugin.Snapshot, error) {
	id := record.ID.String()
	values, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT input_id,value FROM plugin_installation_values WHERE installation_id=?`, id)
	if err != nil {
		return plugin.Snapshot{}, err
	}
	defer values.Close()
	record.Values = map[string]string{}
	for values.Next() {
		var input, value string
		if err := values.Scan(&input, &value); err != nil {
			return plugin.Snapshot{}, err
		}
		record.Values[input] = value
	}
	if err := values.Err(); err != nil {
		return plugin.Snapshot{}, err
	}
	disabled, err := sqlite.Connection(ctx, s.db).QueryContext(ctx, `SELECT kind,name FROM plugin_installation_disabled WHERE installation_id=? ORDER BY kind,name`, id)
	if err != nil {
		return plugin.Snapshot{}, err
	}
	defer disabled.Close()
	for disabled.Next() {
		var kind, name string
		if err := disabled.Scan(&kind, &name); err != nil {
			return plugin.Snapshot{}, err
		}
		switch kind {
		case disabledServer:
			server, err := mcpserver.ParseServerName(name)
			if err != nil {
				return plugin.Snapshot{}, fmt.Errorf("sqlite: installation %s disabled server: %w", id, err)
			}
			record.DisabledServers = append(record.DisabledServers, server)
		case disabledSkill:
			record.DisabledSkills = append(record.DisabledSkills, name)
		default:
			return plugin.Snapshot{}, fmt.Errorf("sqlite: installation %s disabled component kind %q", id, kind)
		}
	}
	if err := disabled.Err(); err != nil {
		return plugin.Snapshot{}, err
	}
	selected, err := s.releases.Get(ctx, record.Selected)
	if err != nil {
		return plugin.Snapshot{}, fmt.Errorf("sqlite: read installation %s selected release: %w", id, err)
	}
	var staged *plugin.Release
	if record.Staged != nil {
		candidate, err := s.releases.Get(ctx, *record.Staged)
		if err != nil {
			return plugin.Snapshot{}, fmt.Errorf("sqlite: read installation %s staged release: %w", id, err)
		}
		staged = &candidate
	}
	installation, err := plugin.Restore(record, selected, staged)
	if err != nil {
		return plugin.Snapshot{}, fmt.Errorf("sqlite: restore installation %s: %w", id, err)
	}
	return plugin.Snapshot{Installation: installation, Selected: selected, Staged: staged}, nil
}

// projectRecord writes only the Host's transaction-bound product index.
func (s *InstallationStore) projectRecord(ctx context.Context, record plugin.Record) error {
	id := record.ID.String()
	var staged any
	if record.Staged != nil {
		staged = record.Staged.String()
	}
	return sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		db := sqlite.Connection(ctx, s.db)
		if _, err := db.ExecContext(ctx, `INSERT INTO plugin_installations(id,source,selected_digest,staged_digest,enabled) VALUES(?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET source=excluded.source,selected_digest=excluded.selected_digest,staged_digest=excluded.staged_digest,enabled=excluded.enabled`,
			id, record.Source, record.Selected.String(), staged, record.Enabled); err != nil {
			return err
		}
		for _, table := range []string{"plugin_installation_values", "plugin_installation_disabled"} {
			if _, err := db.ExecContext(ctx, `DELETE FROM `+table+` WHERE installation_id=?`, id); err != nil {
				return err
			}
		}
		for input, value := range record.Values {
			if _, err := db.ExecContext(ctx, `INSERT INTO plugin_installation_values(installation_id,input_id,value) VALUES(?,?,?)`, id, input, value); err != nil {
				return err
			}
		}
		disabledServers := make([]string, 0, len(record.DisabledServers))
		for _, name := range record.DisabledServers {
			disabledServers = append(disabledServers, name.String())
		}
		for kind, names := range map[string][]string{disabledServer: disabledServers, disabledSkill: record.DisabledSkills} {
			for _, name := range names {
				if _, err := db.ExecContext(ctx, `INSERT INTO plugin_installation_disabled(installation_id,kind,name) VALUES(?,?,?)`, id, kind, name); err != nil {
					return err
				}
			}
		}
		// Server membership is the release catalog's, recorded with the
		// release, so the sources follow it and never a caller-supplied copy.
		if _, err := db.ExecContext(ctx, `DELETE FROM mcp_sources WHERE installation_id=? AND name NOT IN (SELECT name FROM plugin_release_servers WHERE digest=?)`, id, record.Selected.String()); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx, `INSERT INTO mcp_sources(origin, installation_id, name) SELECT ?, ?, name FROM plugin_release_servers WHERE digest=?
		 ON CONFLICT(installation_id, name) WHERE origin = '`+string(mcpserver.OriginInstallation)+`' DO NOTHING`,
			string(mcpserver.OriginInstallation), id, record.Selected.String())
		return err
	})
}

// Initialized records completion separately from the independently durable
// installation operations. An interrupted bootstrap resumes the same identities.
func (s *InstallationStore) Initialized(ctx context.Context) (bool, error) {
	var initialized bool
	err := sqlite.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_initialization WHERE id=1)`).Scan(&initialized)
	return initialized, err
}

func (s *InstallationStore) MarkInitialized(ctx context.Context) error {
	_, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO plugin_initialization(id) VALUES(1) ON CONFLICT(id) DO NOTHING`)
	return err
}

// Stage advances only Flame's proposed update, not the Host's selected release.
func (s *InstallationStore) Stage(ctx context.Context, id identity.InstallationID, digest identity.Digest) error {
	if err := digest.Validate(); err != nil {
		return err
	}
	result, err := sqlite.Connection(ctx, s.db).ExecContext(ctx, `UPDATE plugin_installations SET staged_digest=? WHERE id=?`, digest.String(), id.String())
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err == nil && changed != 1 {
		return plugin.ErrNotFound
	}
	return err
}
