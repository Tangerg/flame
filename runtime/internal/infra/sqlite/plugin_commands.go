package sqlite

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

type PluginCommandStore struct{ db *sql.DB }

func NewPluginCommandStore(db *sql.DB) *PluginCommandStore { return &PluginCommandStore{db: db} }

func (s *PluginCommandStore) Get(ctx context.Context, id string) (*plugin.Command, bool, error) {
	var record plugin.CommandBinding
	var installation, digest, input, contract, server, contribution, invocation, activation string
	err := sqlitebase.Connection(ctx, s.db).QueryRowContext(ctx, `SELECT id,method,business_input_json,contract_json,installation_id,release_digest,server_name,contribution_id,export_name,invocation_json,activation_json,phase FROM plugin_commands WHERE id=?`, id).Scan(&record.ID, &record.Method, &input, &contract, &installation, &digest, &server, &contribution, &record.Export, &invocation, &activation, &record.Phase)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if record.Input, err = jsoninput.Parse([]byte(input)); err != nil {
		return nil, false, err
	}
	if record.Contract, err = jsoninput.Parse([]byte(contract)); err != nil {
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
	if record.Contribution, err = plugin.ParseContributionID(contribution); err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal([]byte(invocation), &record.Invocation, json.RejectUnknownMembers(true)); err != nil {
		return nil, false, err
	}
	if record.Activation, err = decodeCommandActivation(activation); err != nil {
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
	return sqlitebase.RunInTx(ctx, s.db, func(ctx context.Context) error {
		previous, found, err := s.Get(ctx, r.ID)
		if err != nil {
			return err
		}
		if found {
			old := previous.Snapshot()
			if old.Method != r.Method || old.Input != r.Input || old.Contract != r.Contract || old.Provider != r.Provider || old.Server != r.Server || old.Contribution != r.Contribution || old.Export != r.Export || old.Invocation != r.Invocation || !old.Activation.Equal(r.Activation) {
				return plugin.ErrCommandConflict
			}
			_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `UPDATE plugin_commands SET phase=? WHERE id=?`, r.Phase, r.ID)
			return err
		}
		if r.Phase != plugin.CommandPrepared {
			return plugin.ErrInvalid
		}
		invocation, err := json.Marshal(r.Invocation)
		if err != nil {
			return err
		}
		activation, err := encodeCommandActivation(r.Activation)
		if err != nil {
			return err
		}
		_, err = sqlitebase.Connection(ctx, s.db).ExecContext(ctx, `INSERT INTO plugin_commands(id,method,business_input_json,contract_json,installation_id,release_digest,server_name,contribution_id,export_name,invocation_json,activation_json,phase) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.Method, r.Input.String(), r.Contract.String(), r.Provider.InstallationID.String(), r.Provider.Digest.String(), r.Server.String(), r.Contribution.String(), r.Export, string(invocation), activation, r.Phase)
		return err
	})
}

func (s *PluginCommandStore) PendingCommandDependencies(ctx context.Context) ([]plugin.Dependency, error) {
	rows, err := sqlitebase.Connection(ctx, s.db).QueryContext(ctx, `SELECT installation_id,release_digest,activation_json FROM plugin_commands WHERE phase IN (?,?)`, plugin.CommandPrepared, plugin.CommandDispatched)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var held []plugin.Dependency
	for rows.Next() {
		var installation, release, source string
		if err := rows.Scan(&installation, &release, &source); err != nil {
			return nil, err
		}
		activation, err := decodeCommandActivation(source)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(activation.Dependencies, func(dependency plugin.Dependency) bool {
			return dependency.InstallationID.String() == installation && dependency.Digest.String() == release
		}) {
			return nil, plugin.ErrInvalid
		}
		held = append(held, activation.Dependencies...)
	}
	return plugin.CompactDependencies(held), rows.Err()
}

type commandDependencyJSON struct {
	Installation string `json:"installation"`
	Digest       string `json:"digest"`
}

type commandActivationJSON struct {
	Identity     string                  `json:"identity"`
	Dependencies []commandDependencyJSON `json:"dependencies"`
}

func encodeCommandActivation(activation plugin.Activation) (string, error) {
	values := make([]commandDependencyJSON, len(activation.Dependencies))
	for i, dependency := range activation.Dependencies {
		values[i] = commandDependencyJSON{dependency.InstallationID.String(), dependency.Digest.String()}
	}
	encoded, err := json.Marshal(commandActivationJSON{Identity: activation.Identity.String(), Dependencies: values})
	return string(encoded), err
}
func decodeCommandActivation(source string) (plugin.Activation, error) {
	var value commandActivationJSON
	if err := json.Unmarshal([]byte(source), &value, json.RejectUnknownMembers(true)); err != nil {
		return plugin.Activation{}, err
	}
	identity, err := fingerprint.ParseDigest(value.Identity)
	if err != nil {
		return plugin.Activation{}, err
	}
	if len(value.Dependencies) == 0 {
		return plugin.Activation{}, plugin.ErrInvalid
	}
	activation := plugin.Activation{Identity: identity, Dependencies: make([]plugin.Dependency, len(value.Dependencies))}
	for i, dependency := range value.Dependencies {
		id, err := resourceid.ParseInstallation(dependency.Installation)
		if err != nil {
			return plugin.Activation{}, err
		}
		digest, err := fingerprint.ParseDigest(dependency.Digest)
		if err != nil {
			return plugin.Activation{}, err
		}
		activation.Dependencies[i] = plugin.Dependency{InstallationID: id, Digest: digest}
	}
	return activation, activation.Validate()
}
