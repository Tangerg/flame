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

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

type pluginCommandStore struct {
	db     *database
	commit func(context.Context, []byte, func(context.Context) error) error
}

func newPluginCommandStore(db *database, commit func(context.Context, []byte, func(context.Context) error) error) *pluginCommandStore {
	return &pluginCommandStore{db: db, commit: commit}
}

type storedPluginCommandBinding struct {
	Method         string               `json:"method"`
	Input          string               `json:"input"`
	Contract       string               `json:"contract"`
	InstallationID string               `json:"installationId"`
	Digest         string               `json:"digest"`
	Server         string               `json:"server"`
	Contribution   string               `json:"contribution"`
	Export         string               `json:"export"`
	Invocation     framecall.Invocation `json:"invocation"`
	Activation     string               `json:"activation"`
}

func (b storedPluginCommandBinding) provider() (plugin.Dependency, error) {
	installation, err := flameidentity.ParseInstallation(b.InstallationID)
	if err != nil {
		return plugin.Dependency{}, err
	}
	digest, err := framevalue.ParseDigest(b.Digest)
	if err != nil {
		return plugin.Dependency{}, err
	}
	return plugin.Dependency{InstallationID: installation, Digest: digest}, nil
}

func (b storedPluginCommandBinding) restore(id string, phase plugin.CommandPhase) (*plugin.Command, error) {
	record := plugin.CommandBinding{ID: id, Phase: phase, Method: b.Method, Export: b.Export, Invocation: b.Invocation}
	var err error
	if record.Input, err = framevalue.ParseJSON([]byte(b.Input)); err != nil {
		return nil, err
	}
	if record.Contract, err = framevalue.ParseJSON([]byte(b.Contract)); err != nil {
		return nil, err
	}
	if record.Provider, err = b.provider(); err != nil {
		return nil, err
	}
	if record.Server, err = mcpserver.ParseServerName(b.Server); err != nil {
		return nil, err
	}
	if record.Contribution, err = plugin.ParseContributionID(b.Contribution); err != nil {
		return nil, err
	}
	if record.Activation, err = plugins.ReadActivation([]byte(b.Activation)); err != nil {
		return nil, err
	}
	return plugin.RestoreCommand(record)
}

func (s *pluginCommandStore) Get(ctx context.Context, id string) (*plugin.Command, bool, error) {
	return scopedPair(ctx, s.db.read, func(ctx context.Context) (*plugin.Command, bool, error) {
		var phase plugin.CommandPhase
		var binding sql.NullString
		err := s.db.connection(ctx).QueryRowContext(ctx, `SELECT phase,binding_json FROM plugin_commands WHERE id=?`, id).Scan(&phase, &binding)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if !binding.Valid {
			if phase != plugin.CommandRetired {
				return nil, false, plugin.ErrInvalid
			}
			command, err := plugin.NewUnboundRetirement(id)
			return command, err == nil, err
		}
		var record storedPluginCommandBinding
		if err := json.Unmarshal([]byte(binding.String), &record, json.RejectUnknownMembers(true)); err != nil {
			return nil, false, err
		}
		command, err := record.restore(id, phase)
		return command, err == nil, err
	})
}

func (s *pluginCommandStore) Save(ctx context.Context, command *plugin.Command) error {
	if command == nil {
		return plugin.ErrInvalid
	}
	record := command.Snapshot()
	var binding sql.NullString
	if command.HasBinding() {
		if _, err := plugins.DecodeActivation(record.Activation); err != nil {
			return err
		}
		encoded, err := json.Marshal(storedPluginCommandBinding{
			Method:         record.Method,
			Input:          record.Input.String(),
			Contract:       record.Contract.String(),
			InstallationID: record.Provider.InstallationID.String(),
			Digest:         record.Provider.Digest.String(),
			Server:         record.Server.String(),
			Contribution:   record.Contribution.String(),
			Export:         record.Export,
			Invocation:     record.Invocation,
			Activation:     record.Activation.Binding.String(),
		})
		if err != nil {
			return err
		}
		binding = sql.NullString{String: string(encoded), Valid: true}
	}
	encoded, err := json.Marshal(struct {
		Phase   plugin.CommandPhase `json:"phase"`
		Binding string              `json:"binding"`
	}{record.Phase, binding.String})
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
		return s.save(ctx, command, binding)
	})
}

func (s *pluginCommandStore) save(ctx context.Context, command *plugin.Command, binding sql.NullString) error {
	r := command.Snapshot()
	return s.db.write(ctx, func(ctx context.Context) error {
		previous, found, err := s.Get(ctx, r.ID)
		if err != nil {
			return err
		}
		if found {
			if !previous.SameBinding(command) {
				return plugin.ErrCommandConflict
			}
			_, err = s.db.connection(ctx).ExecContext(ctx, `UPDATE plugin_commands SET phase=? WHERE id=?`, r.Phase, r.ID)
			return err
		}
		if r.Phase != plugin.CommandPrepared && (r.Phase != plugin.CommandRetired || binding.Valid) {
			return plugin.ErrInvalid
		}
		_, err = s.db.connection(ctx).ExecContext(ctx, `INSERT INTO plugin_commands(id,binding_json,phase) VALUES(?,?,?)`, r.ID, binding, r.Phase)
		return err
	})
}

func (s *pluginCommandStore) PendingCommandDependencies(ctx context.Context) ([]plugin.Dependency, error) {
	return pendingCommandDependencies(ctx, s.db)
}

func pendingCommandDependencies(ctx context.Context, db *database) ([]plugin.Dependency, error) {
	bindings, err := pendingCommandBindings(ctx, db)
	if err != nil {
		return nil, err
	}
	var held []plugin.Dependency
	for _, binding := range bindings {
		held = append(held, binding.Dependencies...)
	}
	return plugin.CompactDependencies(held), nil
}

func pendingCommandBindings(ctx context.Context, db *database) ([]plugin.Activation, error) {
	return scopedValue(ctx, db.read, func(ctx context.Context) ([]plugin.Activation, error) {
		phases := plugin.PendingCommandPhases()
		args := make([]any, len(phases))
		for index, phase := range phases {
			args[index] = phase
		}
		rows, err := db.connection(ctx).QueryContext(ctx, `SELECT binding_json FROM plugin_commands WHERE phase IN (`+placeholders(len(phases))+`) ORDER BY id`, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var bindings []plugin.Activation
		for rows.Next() {
			var source string
			if err := rows.Scan(&source); err != nil {
				return nil, err
			}
			var binding storedPluginCommandBinding
			if err := json.Unmarshal([]byte(source), &binding, json.RejectUnknownMembers(true)); err != nil {
				return nil, err
			}
			provider, err := binding.provider()
			if err != nil {
				return nil, err
			}
			activation, err := plugins.ReadActivation([]byte(binding.Activation))
			if err != nil {
				return nil, err
			}
			if !slices.Contains(activation.Dependencies, provider) {
				return nil, plugin.ErrInvalid
			}
			bindings = append(bindings, activation)
		}
		return bindings, rows.Err()
	})
}
