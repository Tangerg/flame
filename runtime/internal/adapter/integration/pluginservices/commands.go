package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/dependency"
	"github.com/Tangerg/flame/packages/go-sdk/idempotency"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"

	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type CommandCalls struct {
	publish       invalidation.Publish
	installations plugins.Store
	selections    plugins.SelectionStore
	compositions  plugins.CompositionStore
	pool          *mcp.Pool
	routing       *plugins.Commands
}

func NewCommandCalls(store plugins.Store, selections plugins.SelectionStore, compositions plugins.CompositionStore, pool *mcp.Pool, routing *plugins.Commands, publish invalidation.Publish) (*CommandCalls, error) {
	if dependency.Missing(store) || dependency.Missing(selections) || dependency.Missing(compositions) || pool == nil || routing == nil {
		return nil, errors.New("managed plugin: command routing, installation store and connections are required")
	}
	return &CommandCalls{publish: publish, installations: store, selections: selections, compositions: compositions, pool: pool, routing: routing}, nil
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
		return capability.Target{Scope: capability.ScopeInstallation, Reference: binding.InstallationID.String()}
	})
	if err != nil {
		var zero R
		value = zero
	}
	return commandReply[R]{value: value, binding: reply.binding, session: reply.session}, err
}

func invokeRawCommand(ctx context.Context, c *CommandCalls, method string, admitted jsoninput.Input, validate func(jsoninput.Input) error, accepts func(plugin.CommandBinding) error, resolve func(context.Context) (plugins.ContributionBinding, error), collaborators func(context.Context, plugins.ContributionBinding, *sdk.ClientSession) (sdk.ParamsBase, func(), error), target func(plugins.ContributionBinding) capability.Target) (commandReply[jsoninput.Input], error) {
	var zero commandReply[jsoninput.Input]
	record, err := c.admitCommandRoute(ctx, method, admitted, accepts, resolve, target, false)
	if err != nil {
		return zero, err
	}
	id := record.ID
	ctx, err = pluginsdk.WithInvocation(ctx, record.Invocation)
	if err != nil {
		return zero, err
	}
	if record.Phase != plugin.CommandPrepared {
		defer c.publish.Notify(invalidation.Notice{Resource: invalidation.PluginData})
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
		err = c.routing.Dispatch(ctx, id, c.admitDispatch)
		if err != nil {
			return zero, err
		}
		defer c.publish.Notify(invalidation.Notice{Resource: invalidation.PluginData})
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

// Prepared work may dispatch only with its original activation identity. A receipt
// read deliberately needs only the original owner and never replays that graph.
func (c *CommandCalls) admitDispatch(ctx context.Context, record plugin.CommandBinding) error {
	binding, err := c.original(ctx, record)
	if err != nil {
		return err
	}
	activation, err := c.pool.InstallationActivation(ctx, binding.ServerID)
	if err != nil {
		return err
	}
	if !record.Activation.Equal(activation) {
		return plugin.ErrStale
	}
	return nil
}

// NotifyChange carries no result and cannot advance another owner's data.
func (c *CommandCalls) NotifyChange(ctx context.Context, contribution plugin.ContributionID) error {
	actor, found := operation.ActorFrom(ctx)
	if !found {
		return plugin.ErrUnavailable
	}
	snapshot, found, err := c.installations.Get(ctx, actor.InstallationID())
	if err != nil {
		return err
	}
	if !found || snapshot.Selected.Digest() != actor.Release() || !snapshot.Installation.ServerEnabled(actor.Server()) {
		return plugin.ErrUnavailable
	}
	owner, found := snapshot.Selected.Contribution(contribution)
	if !found || owner.Server != actor.Server() {
		return plugin.ErrUnavailable
	}
	c.publish.Notify(invalidation.Notice{Resource: invalidation.PluginData})
	return nil
}

func (c *CommandCalls) admitCommandRoute(ctx context.Context, method string, admitted jsoninput.Input, accepts func(plugin.CommandBinding) error, resolve func(context.Context) (plugins.ContributionBinding, error), target func(plugins.ContributionBinding) capability.Target, streaming bool) (plugin.CommandBinding, error) {
	id := operation.Identity(ctx)
	capture := func(binding plugins.ContributionBinding) (capability.Invocation, error) {
		return operation.Capture(ctx, target(binding))
	}
	command, found, routeErr := c.routing.Find(ctx, id, method, admitted)
	if routeErr == nil && !found {
		// Discovery is network I/O outside installation admission. Under that
		// boundary, recheck the exact provider before persisting its contract.
		candidate, resolveErr := resolve(ctx)
		if resolveErr != nil {
			return plugin.CommandBinding{}, resolveErr
		}
		session, sessionErr := c.pool.ServiceSession(ctx, candidate.ServerID)
		if sessionErr != nil {
			return plugin.CommandBinding{}, sessionErr
		}
		required, err := capability.Parse(candidate.Contract.JSON())
		if err != nil {
			return plugin.CommandBinding{}, err
		}
		exported, err := pluginsdk.ResolveCapability(ctx, session, candidate.Export, required)
		if err != nil {
			return plugin.CommandBinding{}, err
		}
		binding, err := exported.Method(method)
		if err != nil {
			return plugin.CommandBinding{}, err
		}
		if (len(binding.Descriptor().Stream) != 0) != streaming {
			return plugin.CommandBinding{}, plugin.ErrInvalid
		}
		if binding.Descriptor().Kind != capability.Mutation {
			return plugin.CommandBinding{}, errors.New("managed plugin: command requires a command contract")
		}
		if err := validateInput(ctx, binding, admitted); err != nil {
			return plugin.CommandBinding{}, err
		}
		command, routeErr = c.routing.Bind(ctx, id, method, admitted, func(ctx context.Context) (plugins.CommandAdmission, error) {
			current, err := resolve(ctx)
			if err != nil {
				return plugins.CommandAdmission{}, err
			}
			if current.InstallationID != candidate.InstallationID || current.Digest != candidate.Digest || current.ServerName != candidate.ServerName || current.ServerID != candidate.ServerID || current.ContributionID != candidate.ContributionID || current.Export != candidate.Export || current.Contract != candidate.Contract {
				return plugins.CommandAdmission{}, plugin.ErrUnavailable
			}
			if _, err := c.original(ctx, plugin.CommandBinding{Provider: plugin.Dependency{InstallationID: candidate.InstallationID, Digest: candidate.Digest}, Server: candidate.ServerName, Contribution: candidate.ContributionID, Export: candidate.Export, Contract: candidate.Contract}); err != nil {
				return plugins.CommandAdmission{}, err
			}
			activation, err := c.pool.InstallationActivation(ctx, candidate.ServerID)
			if err != nil {
				return plugins.CommandAdmission{}, err
			}
			return plugins.CommandAdmission{Binding: current, Activation: activation}, nil
		}, capture)
	}
	if errors.Is(routeErr, plugin.ErrCommandConflict) {
		return plugin.CommandBinding{}, fmt.Errorf("managed plugin: %w", errors.Join(idempotency.ErrKeyConflict, routeErr))
	}
	if routeErr != nil {
		return plugin.CommandBinding{}, routeErr
	}
	record := command.Snapshot()
	invocation, err := capture(plugins.ContributionBinding{InstallationID: record.Provider.InstallationID})
	if err != nil {
		return plugin.CommandBinding{}, err
	}
	if invocation != record.Invocation {
		return plugin.CommandBinding{}, errors.Join(idempotency.ErrKeyConflict, plugin.ErrCommandConflict)
	}
	if accepts != nil {
		if err := accepts(record); err != nil {
			return plugin.CommandBinding{}, err
		}
	}
	if record.Phase == plugin.CommandRetired {
		return plugin.CommandBinding{}, plugin.ErrCommandRetired
	}
	return record, nil
}
