package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type CommandCalls struct {
	installations plugins.Store
	selections    plugins.SelectionStore
	pool          *mcpconnection.Pool
	routing       *plugins.Commands
	changed       func(plugin.CommandBinding)
}

func NewCommandCalls(store plugins.Store, selections plugins.SelectionStore, pool *mcpconnection.Pool, routing *plugins.Commands, changed func(plugin.CommandBinding)) (*CommandCalls, error) {
	if dependency.Missing(store) || dependency.Missing(selections) || pool == nil || routing == nil {
		return nil, errors.New("managed plugin: command routing, installation store and connections are required")
	}
	return &CommandCalls{installations: store, selections: selections, pool: pool, routing: routing, changed: changed}, nil
}

type commandReply[T any] struct {
	value   T
	binding plugins.ContributionBinding
	session *sdk.ClientSession
}

func (c *CommandCalls) original(ctx context.Context, record plugin.CommandBinding) (plugins.ContributionBinding, error) {
	snapshot, found, err := c.installations.Get(ctx, record.Provider.InstallationID)
	if err != nil {
		return plugins.ContributionBinding{}, err
	}
	if !found || snapshot.Selected.Digest() != record.Provider.Digest || !snapshot.Installation.ServerEnabled(record.Server) {
		return plugins.ContributionBinding{}, plugin.ErrUnavailable
	}
	id, err := snapshot.Installation.ServerID(record.Server)
	if err != nil {
		return plugins.ContributionBinding{}, err
	}
	contribution, found := snapshot.Selected.Contribution(record.Contribution)
	if !found || contribution.Server != record.Server || contribution.Export != record.Export || contribution.Contract != record.Contract {
		return plugins.ContributionBinding{}, plugin.ErrUnavailable
	}
	return plugins.ContributionBinding{InstallationID: record.Provider.InstallationID, ContributionID: record.Contribution, Digest: record.Provider.Digest, ServerName: record.Server, ServerID: id, Export: record.Export, Contract: record.Contract}, nil
}

func invokeCommand[P, R any](ctx context.Context, c *CommandCalls, required *capability.Contract, method string, input P, validate func(R) error) (commandReply[R], error) {
	return invokeResolvedCommand(ctx, c, method, input, validate, func(ctx context.Context) (plugins.ContributionBinding, error) {
		binding, found, err := plugins.SelectedContribution(ctx, c.installations, c.selections, required)
		if err != nil {
			return binding, err
		}
		if !found {
			return binding, plugin.ErrUnavailable
		}
		return binding, nil
	}, nil)
}
func invokeResolvedCommand[P, R any](ctx context.Context, c *CommandCalls, method string, input P, validate func(R) error, resolve func(context.Context) (plugins.ContributionBinding, error), collaborators func(context.Context, plugins.ContributionBinding, *sdk.ClientSession) (sdk.ParamsBase, func(), error)) (commandReply[R], error) {
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		return commandReply[R]{}, err
	}
	var value R
	reply, err := invokeRawCommand(ctx, c, method, admitted, func(result jsoninput.Input) error {
		if err := json.Unmarshal(result.JSON(), &value, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
		if validate != nil {
			return validate(value)
		}
		return nil
	}, nil, resolve, collaborators, func(binding plugins.ContributionBinding) capability.Target {
		return capability.Target{Scope: "installation", Reference: binding.InstallationID.String()}
	})
	if err != nil {
		var zero R
		value = zero
	}
	return commandReply[R]{value: value, binding: reply.binding, session: reply.session}, err
}

func invokeRawCommand(ctx context.Context, c *CommandCalls, method string, admitted jsoninput.Input, validate func(jsoninput.Input) error, accepts func(plugin.CommandBinding) error, resolve func(context.Context) (plugins.ContributionBinding, error), collaborators func(context.Context, plugins.ContributionBinding, *sdk.ClientSession) (sdk.ParamsBase, func(), error), target func(plugins.ContributionBinding) capability.Target) (commandReply[jsoninput.Input], error) {
	var zero commandReply[jsoninput.Input]
	id := operation.Identity(ctx)
	capture := func(binding plugins.ContributionBinding) (capability.Invocation, error) {
		return operation.Capture(ctx, target(binding))
	}
	command, found, err := c.routing.Find(ctx, id, method, admitted)
	if err == nil && !found {
		// Discovery is network I/O outside installation admission. Under that
		// boundary, recheck the exact provider before persisting its contract.
		candidate, resolveErr := resolve(ctx)
		if resolveErr != nil {
			return zero, resolveErr
		}
		session, sessionErr := c.pool.ServiceSession(ctx, candidate.ServerID)
		if sessionErr != nil {
			return zero, sessionErr
		}
		required, err := capability.Parse(candidate.Contract.JSON())
		if err != nil {
			return zero, err
		}
		exported, err := pluginsdk.ResolveCapability(ctx, session, candidate.Export, required)
		if err != nil {
			return zero, err
		}
		binding, err := exported.Method(method)
		if err != nil {
			return zero, err
		}
		if binding.Descriptor().Kind != capability.Mutation {
			return zero, errors.New("managed plugin: command requires a command contract")
		}
		if err := binding.ValidateInput(admitted); err != nil {
			return zero, err
		}
		command, err = c.routing.Bind(ctx, id, method, admitted, func(ctx context.Context) (plugins.ContributionBinding, error) {
			current, err := resolve(ctx)
			if err != nil {
				return current, err
			}
			if current.InstallationID != candidate.InstallationID || current.Digest != candidate.Digest || current.ServerName != candidate.ServerName || current.ServerID != candidate.ServerID || current.ContributionID != candidate.ContributionID || current.Export != candidate.Export || current.Contract != candidate.Contract {
				return current, plugin.ErrUnavailable
			}
			if _, err := c.original(ctx, plugin.CommandBinding{Provider: plugin.Dependency{InstallationID: candidate.InstallationID, Digest: candidate.Digest}, Server: candidate.ServerName, Contribution: candidate.ContributionID, Export: candidate.Export, Contract: candidate.Contract}); err != nil {
				return current, err
			}
			return current, nil
		}, capture)
	}
	if errors.Is(err, plugin.ErrCommandConflict) {
		return zero, fmt.Errorf("managed plugin: %w", errors.Join(idempotency.ErrKeyConflict, err))
	}
	if err != nil {
		return zero, err
	}
	record := command.Snapshot()
	invocation, err := capture(plugins.ContributionBinding{InstallationID: record.Provider.InstallationID})
	if err != nil {
		return zero, err
	}
	if invocation != record.Invocation {
		return zero, errors.Join(idempotency.ErrKeyConflict, plugin.ErrCommandConflict)
	}
	if accepts != nil {
		if err := accepts(record); err != nil {
			return zero, err
		}
	}
	ctx, err = pluginsdk.WithInvocation(ctx, record.Invocation)
	if err != nil {
		return zero, err
	}
	if record.Phase != plugin.CommandPrepared && c.changed != nil {
		defer c.changed(record)
	}
	binding, err := c.original(ctx, record)
	if err != nil {
		if record.Phase != plugin.CommandPrepared {
			err = errors.Join(plugin.ErrCommandUnconfirmed, err)
		}
		return zero, err
	}
	session, err := c.pool.ServiceSession(ctx, binding.ServerID)
	if err != nil {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err)
	}
	contract, err := capability.Parse(record.Contract.JSON())
	if err != nil {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err)
	}
	exported, err := pluginsdk.NewCapabilityBinding(session, record.Export, contract)
	if err != nil {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err)
	}
	bound, err := exported.Method(method)
	if err != nil || bound.Descriptor().Kind != capability.Mutation {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err, plugin.ErrInvalid)
	}
	var result jsoninput.Input
	if record.Phase == plugin.CommandPrepared {
		metadata := sdk.ParamsBase{}
		if collaborators != nil {
			var release func()
			metadata, release, err = collaborators(ctx, binding, session)
			if err != nil {
				return zero, err
			}
			defer release()
		}
		err = c.routing.Dispatch(ctx, id, func(ctx context.Context, record plugin.CommandBinding) error {
			_, err := c.original(ctx, record)
			return err
		})
		if err != nil {
			return zero, err
		}
		if c.changed != nil {
			defer c.changed(record)
		}
		result, err = bound.InvokeWithMetadata(ctx, id, record.Input, metadata)
	} else {
		var found bool
		result, found, err = bound.ReadReceipt(ctx, id, record.Input)
		if !found && err == nil {
			err = plugin.ErrCommandUnconfirmed
		}
	}
	_, known := errors.AsType[*capability.Refusal](err)
	if err != nil && !known {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err)
	}
	if err == nil && validate != nil {
		if validationErr := validate(result); validationErr != nil {
			return zero, errors.Join(plugin.ErrCommandUnconfirmed, validationErr)
		}
	}
	if observeErr := c.routing.Observe(ctx, id); observeErr != nil {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, observeErr)
	}
	return commandReply[jsoninput.Input]{value: result, binding: binding, session: session}, err
}
