package pluginservices

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
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
	return plugins.ServiceBinding{InstallationID: record.Provider.InstallationID, Digest: record.Provider.Digest, ServerName: record.Server, ServerID: id}, nil
}

func invokeCommand[P, R any](ctx context.Context, c *CommandCalls, kind plugin.ServiceKind, method string, input P, validate func(R) error) (commandReply[R], error) {
	var zero commandReply[R]
	id := operation.Identity(ctx)
	digest, err := pluginsdk.InputDigest(input)
	if err != nil {
		return zero, err
	}
	fingerprint, err := fingerprint.ParseDigest(digest)
	if err != nil {
		return zero, err
	}
	command, err := c.routing.Bind(ctx, id, method, fingerprint, func(ctx context.Context) (plugins.ServiceBinding, error) {
		binding, found, err := plugins.ServiceOwner(ctx, c.installations, kind)
		if err != nil {
			return binding, err
		}
		if !found {
			return binding, plugin.ErrUnavailable
		}
		return binding, nil
	})
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
	var value R
	if record.Phase == plugin.CommandPrepared {
		err = c.routing.Dispatch(ctx, id, func(ctx context.Context, record plugin.CommandBinding) error {
			_, err := c.original(ctx, record)
			return err
		})
		if err != nil {
			return zero, err
		}
		value, err = pluginsdk.CallCommand[P, R](ctx, session, method, id, input)
	} else {
		var found bool
		value, found, err = pluginsdk.ReadReceipt[P, R](ctx, session, id, method, input)
		if !found && err == nil {
			err = plugin.ErrCommandUnconfirmed
		}
	}
	_, known := errors.AsType[*pluginsdk.Failure](err)
	if err != nil && !known {
		return zero, errors.Join(plugin.ErrCommandUnconfirmed, err)
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
