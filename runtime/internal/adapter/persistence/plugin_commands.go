package persistence

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"slices"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"
)

type pluginCommandStore struct {
	db     *database
	commit func(context.Context, []byte, func(context.Context) error) error
}

func newPluginCommandStore(db *database, commit func(context.Context, []byte, func(context.Context) error) error) *pluginCommandStore {
	return &pluginCommandStore{db: db, commit: commit}
}
func (s *pluginCommandStore) Get(ctx context.Context, id string) (*plugin.Command, bool, error) {
	return scopedPair(ctx, s.db.read, func(ctx context.Context) (*plugin.Command, bool, error) {
		var record plugin.CommandBinding
		var installation, digest, input, contract, server, contribution, invocation, activation string
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT id,method,business_input_json,contract_json,installation_id,release_digest,server_name,contribution_id,export_name,invocation_json,activation_json,phase FROM plugin_commands WHERE id=?`, id).Scan(&record.ID, &record.Method, &input, &contract, &installation, &digest, &server, &contribution, &record.Export, &invocation, &activation, &record.Phase)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if record.Input, err = framevalue.ParseJSON([]byte(input)); err != nil {
			return nil, false, err
		}
		if record.Contract, err = framevalue.ParseJSON([]byte(contract)); err != nil {
			return nil, false, err
		}
		if record.Provider.InstallationID, err = flameidentity.ParseInstallation(installation); err != nil {
			return nil, false, err
		}
		if record.Provider.Digest, err = framevalue.ParseDigest(digest); err != nil {
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
	})
}
func (s *pluginCommandStore) Save(ctx context.Context, command *plugin.Command) error {
	if command == nil || s.commit == nil {
		return plugin.ErrInvalid
	}
	record := command.Snapshot()
	if _, err := plugins.DecodeActivation(record.Activation); err != nil {
		return err
	}
	encoded, err := json.Marshal(struct {
		ID           string              `json:"id"`
		Method       string              `json:"method"`
		Input        string              `json:"input"`
		Contract     string              `json:"contract"`
		Installation string              `json:"installation"`
		Release      string              `json:"release"`
		Server       string              `json:"server"`
		Contribution string              `json:"contribution"`
		Export       string              `json:"export"`
		Invocation   framemcp.Invocation `json:"invocation"`
		Binding      string              `json:"binding"`
		Phase        string              `json:"phase"`
	}{record.ID, record.Method, record.Input.String(), record.Contract.String(), record.Provider.InstallationID.String(), record.Provider.Digest.String(), record.Server.String(), record.Contribution.String(), record.Export, record.Invocation, record.Activation.Binding.String(), string(record.Phase)})
	if err != nil {
		return err
	}
	request, err := json.Marshal(struct {
		Kind     string            `json:"kind"`
		Identity string            `json:"identity"`
		Digest   framevalue.Digest `json:"digest"`
	}{"plugin.command", record.ID, framevalue.Hash(encoded)})
	if err != nil {
		return err
	}
	return s.commit(ctx, request, func(ctx context.Context) error {
		return s.save(ctx, command)
	})
}
func (s *pluginCommandStore) save(ctx context.Context, command *plugin.Command) error {
	if command == nil {
		return plugin.ErrInvalid
	}
	r := command.Snapshot()
	return s.db.write(ctx, func(ctx context.Context) error {
		previous, found, err := s.Get(ctx, r.ID)
		if err != nil {
			return err
		}
		if found {
			old := previous.Snapshot()
			if old.Method != r.Method || old.Input != r.Input || old.Contract != r.Contract || old.Provider != r.Provider || old.Server != r.Server || old.Contribution != r.Contribution || old.Export != r.Export || old.Invocation != r.Invocation || !old.Activation.Equal(r.Activation) {
				return plugin.ErrCommandConflict
			}
			_, err = s.db.connection(ctx).ExecContext(ctx, `UPDATE plugin_commands SET phase=? WHERE id=?`, r.Phase, r.ID)
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
		_, err = s.db.connection(ctx).ExecContext(ctx, `INSERT INTO plugin_commands(id,method,business_input_json,contract_json,installation_id,release_digest,server_name,contribution_id,export_name,invocation_json,activation_json,phase) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.Method, r.Input.String(), r.Contract.String(), r.Provider.InstallationID.String(), r.Provider.Digest.String(), r.Server.String(), r.Contribution.String(), r.Export, string(invocation), activation, r.Phase)
		return err
	})
}
func (s *pluginCommandStore) PendingCommandDependencies(ctx context.Context) ([]plugin.Dependency, error) {
	bindings, err := s.pendingBindings(ctx)
	if err != nil {
		return nil, err
	}
	var held []plugin.Dependency
	for _, binding := range bindings {
		held = append(held, binding.Dependencies...)
	}
	return plugin.CompactDependencies(held), nil
}
func (s *pluginCommandStore) pendingBindings(ctx context.Context) ([]plugin.Activation, error) {
	return scopedValue(ctx, s.db.read, func(ctx context.Context) ([]plugin.Activation, error) {
		rows, err := s.db.connection(ctx).QueryContext(ctx, `SELECT installation_id,release_digest,activation_json FROM plugin_commands WHERE phase IN (?,?) ORDER BY id`, plugin.CommandPrepared, plugin.CommandDispatched)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var bindings []plugin.Activation
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
			bindings = append(bindings, activation)
		}
		return bindings, rows.Err()
	})
}
func encodeCommandActivation(activation plugin.Activation) (string, error) {
	if _, err := plugins.DecodeActivation(activation); err != nil {
		return "", err
	}
	return activation.Binding.String(), nil
}
func decodeCommandActivation(source string) (plugin.Activation, error) {
	return plugins.ReadActivation([]byte(source))
}
