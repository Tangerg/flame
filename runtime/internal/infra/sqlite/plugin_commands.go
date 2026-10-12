package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

type PluginCommandStore struct{ db *sql.DB }

func NewPluginCommandStore(db *sql.DB) *PluginCommandStore { return &PluginCommandStore{db: db} }

func (s *PluginCommandStore) Get(ctx context.Context, id string) (*plugin.Command, bool, error) {
	var record plugin.CommandBinding
	var installation, digest, input, server string
	err := Connection(ctx, s.db).QueryRowContext(ctx, `SELECT id,method,input_digest,installation_id,release_digest,server_name,phase FROM plugin_commands WHERE id=?`, id).Scan(&record.ID, &record.Method, &input, &installation, &digest, &server, &record.Phase)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if record.Input, err = fingerprint.ParseDigest(input); err != nil {
		return nil, false, err
	}
	if record.Provider.InstallationID, err = resourceid.ParseInstallation(installation); err != nil {
		return nil, false, err
	}
	if record.Provider.Digest, err = fingerprint.ParseDigest(digest); err != nil {
		return nil, false, err
	}
	if record.Server, err = mcpserver.ParseServerName(server); err != nil {
		return nil, false, err
	}
	command, err := plugin.RestoreCommand(record)
	return command, err == nil, err
}

func (s *PluginCommandStore) Save(ctx context.Context, command *plugin.Command) error {
	if command == nil {
		return plugin.ErrInvalid
	}
	r := command.Snapshot()
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		previous, found, err := s.Get(ctx, r.ID)
		if err != nil {
			return err
		}
		if found {
			old := previous.Snapshot()
			if old.Method != r.Method || old.Input != r.Input || old.Provider != r.Provider || old.Server != r.Server {
				return plugin.ErrCommandConflict
			}
			_, err = Connection(ctx, s.db).ExecContext(ctx, `UPDATE plugin_commands SET phase=? WHERE id=?`, r.Phase, r.ID)
			return err
		}
		if r.Phase != plugin.CommandPrepared {
			return plugin.ErrInvalid
		}
		_, err = Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO plugin_commands(id,method,input_digest,installation_id,release_digest,server_name,phase) VALUES(?,?,?,?,?,?,?)`, r.ID, r.Method, r.Input.String(), r.Provider.InstallationID.String(), r.Provider.Digest.String(), r.Server.String(), r.Phase)
		return err
	})
}

func (s *PluginCommandStore) PendingCommandDependencies(ctx context.Context) ([]plugin.Dependency, error) {
	rows, err := Connection(ctx, s.db).QueryContext(ctx, `SELECT DISTINCT installation_id,release_digest FROM plugin_commands WHERE phase != 'observed' ORDER BY installation_id,release_digest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var held []plugin.Dependency
	for rows.Next() {
		var installation, digest string
		if err := rows.Scan(&installation, &digest); err != nil {
			return nil, err
		}
		id, err := resourceid.ParseInstallation(installation)
		if err != nil {
			return nil, err
		}
		release, err := fingerprint.ParseDigest(digest)
		if err != nil {
			return nil, err
		}
		held = append(held, plugin.Dependency{InstallationID: id, Digest: release})
	}
	return held, rows.Err()
}
