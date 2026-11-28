package main

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/methods/internal/contract"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func register(server *sdk.Server, c *connector) (*pluginmcp.Backend, error) {
	methods, err := contract.New()
	if err != nil {
		return nil, err
	}
	capability, err := methods.Capability()
	if err != nil {
		return nil, err
	}
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		registerQuery(backend, methods.Board, c.board),
		registerQuery(backend, methods.Read, c.read),
		registerCommand(backend, c, methods.Save, c.save),
		registerCommand(backend, c, methods.Start, c.begin),
		registerCommand(backend, c, methods.Approve, c.approve),
		pluginstorage.RegisterReceipts(backend, c.store.db),
		backend.BindCapability(contract.Export, capability),
		backend.OnSetup(c.setup),
	); err != nil {
		return nil, err
	}
	return backend, backend.Publish()
}

func human(ctx context.Context) error {
	call, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return err
	}
	if call.Origin != extension.Human {
		return refuse("this method operation requires a person")
	}
	return nil
}

func registerQuery[P, R any](backend *pluginmcp.Backend, method *pluginmcp.Method[P, R], query func(context.Context, P) (R, error)) error {
	return method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, cmd pluginmcp.Command[P]) (R, error) {
		if err := human(ctx); err != nil {
			var zero R
			return zero, err
		}
		return query(ctx, cmd.Input)
	})
}

func registerCommand[P, R any](backend *pluginmcp.Backend, c *connector, method *pluginmcp.Method[P, R], prepare func(context.Context, P, string) (pluginstorage.CommandPlan[R], error)) error {
	return method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, cmd pluginmcp.Command[P]) (R, error) {
		var zero R
		if err := human(ctx); err != nil {
			return zero, err
		}
		body, err := cmd.RequestInput()
		if err != nil {
			return zero, err
		}
		out, err := pluginstorage.Command(ctx, c.store.db, method.Name(), string(cmd.ID), body, classify, func(ctx context.Context, input P) (pluginstorage.CommandPlan[R], error) {
			return prepare(ctx, input, string(cmd.ID))
		})
		if err == nil {
			c.signal()
		}
		return out, err
	})
}
