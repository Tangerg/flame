package main

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/opencode/internal/contract"
	"github.com/Tangerg/flame/plugins/opencode/internal/wire"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newServer(c *connector) (*sdk.Server, error) {
	methods, err := contract.New()
	if err != nil {
		return nil, err
	}
	capability, err := methods.Capability()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.opencode", Version: "1.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	err = errors.Join(
		pluginstorage.RegisterReceipts(backend, c.db),
		methods.List.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.ListRequest]) (wire.Sessions, error) {
			if err := requireHuman(ctx); err != nil {
				return wire.Sessions{}, err
			}
			return c.list(ctx, command.Input)
		}),
		methods.Get.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.SessionRequest]) (wire.Session, error) {
			if err := requireHuman(ctx); err != nil {
				return wire.Session{}, err
			}
			return c.get(ctx, command.Input)
		}),
		registerExternal(backend, c, methods.Open, true, c.open),
		registerExternal(backend, c, methods.Fork, false, c.fork),
		registerLocal(backend, c, methods.Prompt, c.prompt),
		registerLocal(backend, c, methods.Cancel, c.cancel),
		registerLocal(backend, c, methods.Permission, c.answer),
		backend.BindCapability(contract.Export, capability),
	)
	if err != nil {
		return nil, err
	}
	return server, backend.Publish()
}

func requireHuman(ctx context.Context) error {
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return err
	}
	if invocation.Origin != extension.Human {
		return refuse("native sessions require a direct user action")
	}
	return nil
}

func registerLocal[P any](backend *pluginmcp.Backend, c *connector, method *pluginmcp.Method[P, wire.Session], prepare sessionAction[P]) error {
	return method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[P]) (wire.Session, error) {
		if err := requireHuman(ctx); err != nil {
			return wire.Session{}, err
		}
		return localCommand(c, ctx, method.Name(), command, prepare)
	})
}

func registerExternal[P any](backend *pluginmcp.Backend, c *connector, method *pluginmcp.Method[P, wire.Session], replayable bool, dispatch func(context.Context, P) (wire.Session, error)) error {
	return method.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[P]) (wire.Session, error) {
		if err := requireHuman(ctx); err != nil {
			return wire.Session{}, err
		}
		input, err := command.RequestInput()
		if err != nil {
			return wire.Session{}, err
		}
		c.actions.Lock()
		defer c.actions.Unlock()
		prepared, original, err := pluginstorage.AdmitCommand(ctx, c.db, method.Name(), string(command.ID), input, nil)
		if err != nil {
			return wire.Session{}, err
		}
		if prepared == nil {
			return pluginmcp.DecodeReply[wire.Session](original)
		}
		if !prepared.NewlyAdmitted() && !replayable {
			return wire.Session{}, pluginmcp.ErrOutcomeUnconfirmed
		}
		result, effectErr := dispatch(ctx, command.Input)
		return pluginstorage.CompleteExternalCommand(ctx, prepared, classify, func(context.Context) (pluginstorage.CommandPlan[wire.Session], error) {
			if effectErr != nil {
				return pluginstorage.CommandPlan[wire.Session]{}, effectErr
			}
			return pluginstorage.NewCommandPlan(result, nil)
		})
	})
}
