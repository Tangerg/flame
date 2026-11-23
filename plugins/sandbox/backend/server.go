package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/sandbox/internal/contract"
	"github.com/Tangerg/flame/plugins/sandbox/internal/sandbox"
	"github.com/Tangerg/flame/plugins/sandbox/internal/wire"
	sdk "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/scope/core/tool"
)

const executeTool = "sandbox_execute"

func newServer(runner *runner) (*sdk.Server, error) {
	if runner == nil {
		return nil, errors.New("sandbox: execution owner is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.sandbox", Version: "1.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	execute, err := tool.NewFunc(tool.FuncConfig{Name: executeTool, Description: contracts.Execute.Descriptor().Description}, func(ctx context.Context, _ wire.ExecuteInput) (sandbox.Result, error) {
		id, err := pluginsdk.ToolCommandID(ctx)
		if err != nil {
			return sandbox.Result{}, err
		}
		original, err := pluginmcp.RequestInput(ctx)
		if err != nil {
			return sandbox.Result{}, err
		}
		result, err := runner.execute(ctx, contracts.Execute.Name(), id, original)
		return result, pluginsdk.RefusedToolFailure(err)
	})
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, execute); err != nil {
		return nil, err
	}
	toolReceipts, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, runner.db),
		contracts.Execute.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.ExecuteInput]) (sandbox.Result, error) {
			original, err := command.RequestInput()
			if err != nil {
				return sandbox.Result{}, err
			}
			return runner.execute(ctx, contracts.Execute.Name(), string(command.ID), original)
		}),
		contracts.Limits.RegisterServer(backend, classify, func(context.Context, *sdk.ServerSession, pluginmcp.Command[struct{}]) (wire.Limits, error) {
			return runner.limits(), nil
		}),
		toolReceipts.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[pluginmcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			input := command.Input
			if input.Method != executeTool {
				return pluginsdk.ToolReceipt{}, nil
			}
			input.Method = contracts.Execute.Name()
			receipt, err := pluginstorage.ReadReceipt(ctx, runner.db, input)
			if err != nil || !receipt.Found {
				return pluginsdk.ToolReceipt{}, err
			}
			result, err := readOutcome(receipt.Reply)
			return pluginsdk.ToolReceiptFrom(result, err, pluginsdk.RefusedToolFailure)
		}),
		backend.BindCapability("sandbox", contracts.Capability),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}
