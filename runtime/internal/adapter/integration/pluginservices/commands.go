package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

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
	pool          *mcpconnection.Pool
	routing       *plugins.Commands
}

func NewCommandCalls(store plugins.Store, pool *mcpconnection.Pool, routing *plugins.Commands) (*CommandCalls, error) {
	if dependency.Missing(store) || pool == nil || routing == nil {
		return nil, errors.New("managed plugin: command routing, installation store and connections are required")
	}
	return &CommandCalls{installations: store, pool: pool, routing: routing}, nil
}

type commandReply[T any] struct {
	value   T
	binding plugins.ServiceBinding
	session *sdk.ClientSession
}

func (c *CommandCalls) original(ctx context.Context, record plugin.CommandBinding) (plugins.ServiceBinding, error) {
	snapshot, found, err := c.installations.Get(ctx, record.Provider.InstallationID)
	if err != nil {
		return plugins.ServiceBinding{}, err
	}
	if !found || snapshot.Selected.Digest() != record.Provider.Digest || !snapshot.Installation.ServerEnabled(record.Server) {
		return plugins.ServiceBinding{}, plugin.ErrUnavailable
	}
	id, err := snapshot.Installation.ServerID(record.Server)
	if err != nil {
		return plugins.ServiceBinding{}, err
	}
	return plugins.ServiceBinding{InstallationID: record.Provider.InstallationID, Digest: record.Provider.Digest, ServerName: record.Server, ServerID: id, Contract: record.Contract}, nil
}

func invokeCommand[P, R any](ctx context.Context, c *CommandCalls, kind plugin.ServiceKind, method string, input P, validate func(R) error) (commandReply[R], error) {
	return invokeResolvedCommand(ctx, c, method, input, validate, func(ctx context.Context) (plugins.ServiceBinding, error) {
		binding, found, err := plugins.ServiceOwner(ctx, c.installations, kind)
		if err != nil {
			return binding, err
		}
		if !found {
			return binding, plugin.ErrUnavailable
		}
		return binding, nil
	})
}
func invokeResolvedCommand[P, R any](ctx context.Context, c *CommandCalls, method string, input P, validate func(R) error, resolve func(context.Context) (plugins.ServiceBinding, error)) (commandReply[R], error) {
	var zero commandReply[R]
	id := operation.Identity(ctx)
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		return zero, err
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
		binding, bindingErr := pluginsdk.Resolve(ctx, session, method)
		if bindingErr != nil {
			return zero, bindingErr
		}
		if binding.Descriptor().Kind != pluginsdk.Mutation {
			return zero, errors.New("managed plugin: command requires a command contract")
		}
		if err := binding.ValidateInput(admitted); err != nil {
			return zero, err
		}
		command, err = c.routing.Bind(ctx, id, method, admitted, func(ctx context.Context) (plugins.ServiceBinding, error) {
			current, err := resolve(ctx)
			if err != nil {
				return current, err
			}
			if current.InstallationID != candidate.InstallationID || current.Digest != candidate.Digest || current.ServerName != candidate.ServerName || current.ServerID != candidate.ServerID {
				return current, plugin.ErrUnavailable
			}
			if _, err := c.original(ctx, plugin.CommandBinding{Provider: plugin.Dependency{InstallationID: candidate.InstallationID, Digest: candidate.Digest}, Server: candidate.ServerName}); err != nil {
				return current, err
			}
			current.Contract = binding.Contract()
			return current, nil
		})
	}
	if errors.Is(err, plugin.ErrCommandConflict) {
		return zero, fmt.Errorf("managed plugin: %w", errors.Join(idempotency.ErrKeyConflict, err))
	}
	if err != nil {
		return zero, err
	}
	record := command.Snapshot()
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
	var descriptor pluginsdk.MethodDescriptor
	if err := json.Unmarshal(record.Contract.JSON(), &descriptor, json.RejectUnknownMembers(true)); err != nil {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err)
	}
	bound, err := pluginsdk.NewBinding(session, descriptor)
	if err != nil || descriptor.Name != method || descriptor.Kind != pluginsdk.Mutation {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err, plugin.ErrInvalid)
	}
	if bound.Contract() != record.Contract {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, plugin.ErrInvalid)
	}
	var value R
	var result jsoninput.Input
	if record.Phase == plugin.CommandPrepared {
		err = c.routing.Dispatch(ctx, id, func(ctx context.Context, record plugin.CommandBinding) error {
			_, err := c.original(ctx, record)
			return err
		})
		if err != nil {
			return zero, err
		}
		result, err = bound.Invoke(ctx, id, record.Input)
	} else {
		var found bool
		result, found, err = bound.ReadReceipt(ctx, id, record.Input)
		if !found && err == nil {
			err = plugin.ErrCommandUnconfirmed
		}
	}
	_, known := errors.AsType[*pluginsdk.Failure](err)
	if err != nil && !known {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err)
	}
	if err == nil {
		if decodeErr := json.Unmarshal(result.JSON(), &value, json.RejectUnknownMembers(true)); decodeErr != nil {
			return zero, errors.Join(plugin.ErrCommandUnconfirmed, decodeErr)
		}
	}
	if err == nil && validate != nil {
		if validationErr := validate(value); validationErr != nil {
			return zero, errors.Join(plugin.ErrCommandUnconfirmed, validationErr)
		}
	}
	if observeErr := c.routing.Observe(ctx, id); observeErr != nil {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, observeErr)
	}
	return commandReply[R]{value: value, binding: binding, session: session}, err
}
