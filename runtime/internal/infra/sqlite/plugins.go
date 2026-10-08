package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type InstallationStore struct{ db *sql.DB }

func NewInstallationStore(db *sql.DB) *InstallationStore { return &InstallationStore{db: db} }

const (
	disabledServer = "server"
	disabledSkill  = "skill"
)

func (s *InstallationStore) List(ctx context.Context) ([]plugin.Snapshot, error) {
	var result []plugin.Snapshot
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		records, err := s.records(ctx, `SELECT id,source,selected_digest,staged_digest,admission_state FROM plugin_installations ORDER BY id`)
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

func (s *InstallationStore) Get(ctx context.Context, id resourceid.InstallationID) (plugin.Snapshot, error) {
	var result plugin.Snapshot
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		records, err := s.records(ctx, `SELECT id,source,selected_digest,staged_digest,admission_state FROM plugin_installations WHERE id=?`, id.String())
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return plugin.ErrNotFound
		}
		result, err = s.restore(ctx, records[0])
		return err
	})
	return result, err
}

func (s *InstallationStore) records(ctx context.Context, query string, args ...any) ([]plugin.Record, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []plugin.Record
	for rows.Next() {
		var record plugin.Record
		var id, selected, state string
		var staged sql.NullString
		if err := rows.Scan(&id, &record.Source, &selected, &staged, &state); err != nil {
			return nil, err
		}
		record.State = plugin.State(state)
		if record.ID, err = resourceid.ParseInstallation(id); err != nil {
			return nil, fmt.Errorf("sqlite: decode installation identity: %w", err)
		}
		if record.Selected, err = fingerprint.ParseDigest(selected); err != nil {
			return nil, fmt.Errorf("sqlite: decode selected release: %w", err)
		}
		if staged.Valid {
			digest, err := fingerprint.ParseDigest(staged.String)
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
	values, err := conn(ctx, s.db).QueryContext(ctx, `SELECT input_id,value FROM plugin_installation_values WHERE installation_id=?`, id)
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
	disabled, err := conn(ctx, s.db).QueryContext(ctx, `SELECT kind,name FROM plugin_installation_disabled WHERE installation_id=? ORDER BY kind,name`, id)
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
	installation, err := plugin.Restore(record)
	if err != nil {
		return plugin.Snapshot{}, err
	}
	catalog := NewReleaseStore(s.db)
	selected, err := catalog.Get(ctx, record.Selected)
	if err != nil {
		return plugin.Snapshot{}, fmt.Errorf("sqlite: read installation %s selected release: %w", id, err)
	}
	result := plugin.Snapshot{Installation: installation, Selected: selected}
	if record.Staged != nil {
		staged, err := catalog.Get(ctx, *record.Staged)
		if err != nil {
			return plugin.Snapshot{}, fmt.Errorf("sqlite: read installation %s staged release: %w", id, err)
		}
		result.Staged = &staged
	}
	return result, nil
}

// Save replaces the installation's state and keeps one MCP source row per
// server its selected release declares. A source the release no longer
// declares is removed, and its exposure, credentials and standing approvals
// follow by cascade.
func (s *InstallationStore) Save(ctx context.Context, installation *plugin.Installation) error {
	record := installation.Snapshot()
	id := record.ID.String()
	var staged any
	if record.Staged != nil {
		staged = record.Staged.String()
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		db := conn(ctx, s.db)
		if _, err := db.ExecContext(ctx, `INSERT INTO plugin_installations(id,source,selected_digest,staged_digest,admission_state) VALUES(?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET source=excluded.source,selected_digest=excluded.selected_digest,staged_digest=excluded.staged_digest,admission_state=excluded.admission_state`,
			id, record.Source, record.Selected.String(), staged, string(record.State)); err != nil {
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

// Remove deletes the installation; its state, MCP sources and everything that
// references them follow by cascade.
func (s *InstallationStore) Remove(ctx context.Context, id resourceid.InstallationID) error {
	result, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM plugin_installations WHERE id=?`, id.String())
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
