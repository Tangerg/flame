package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/a2a/internal/contract"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"

	"github.com/Tangerg/frame"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/tool"
)

const (
	agentsTool = "a2a_agents"
	statusTool = "a2a_status"
	sendTool   = "a2a_send"
	cancelTool = "a2a_cancel"
)

const capabilityExport = "remote-agents"

func newServer(connector *connector) (*sdk.Server, error) {
	if connector == nil {
		return nil, errors.New("a2a: connector is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.a2a", Version: "1.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	tools, err := connector.tools(contracts)
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, tools...); err != nil {
		return nil, err
	}
	if err := connector.registerMethods(backend, contracts); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

func (c *connector) tools(contracts rpc.Contracts) ([]tool.Tool, error) {
	agents, err := tool.NewFunc(tool.FuncConfig{Name: agentsTool, Description: contracts.Agents.Descriptor().Description}, c.list)
	if err != nil {
		return nil, err
	}
	status, err := tool.NewFunc(tool.FuncConfig{Name: statusTool, Description: contracts.Status.Descriptor().Description}, c.statusTool)
	if err != nil {
		return nil, err
	}
	send, err := commandTool[wire.SendRequest](sendTool, contracts.Send.Descriptor(), c.send)
	if err != nil {
		return nil, err
	}
	cancel, err := commandTool[wire.TaskRequest](cancelTool, contracts.Cancel.Descriptor(), c.cancel)
	if err != nil {
		return nil, err
	}
	return []tool.Tool{agents, send, status, cancel}, nil
}

func (c *connector) statusTool(ctx context.Context, input wire.TaskRequest) (wire.Result, error) {
	result, err := c.status(ctx, input)
	if err == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return result, errors.Join(err, ctx.Err(), context.Cause(ctx))
	}
	kind := tool.FailureKindFailed
	if _, refused := errors.AsType[*framecall.Refusal](err); refused {
		kind = tool.FailureKindRejected
	}
	return result, tool.FailureFromCause(kind, err)
}

func (c *connector) registerMethods(backend *pluginmcp.Backend, contracts rpc.Contracts) error {
	receipts, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return err
	}
	return errors.Join(
		pluginstorage.RegisterReceipts(backend, c.db),
		contracts.Agents.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[struct{}]) (wire.Agents, error) {
			return c.list(ctx, command.Input)
		}),
		contracts.Send.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.SendRequest]) (wire.Result, error) {
			input, err := command.RequestInput()
			if err != nil {
				return wire.Result{}, err
			}
			return c.send(ctx, contracts.Send.Name(), string(command.ID), input)
		}),
		contracts.Status.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.TaskRequest]) (wire.Result, error) {
			return c.status(ctx, command.Input)
		}),
		contracts.Cancel.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.TaskRequest]) (wire.Result, error) {
			input, err := command.RequestInput()
			if err != nil {
				return wire.Result{}, err
			}
			return c.cancel(ctx, contracts.Cancel.Name(), string(command.ID), input)
		}),
		receipts.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[pluginmcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			input := command.Input
			switch input.Method {
			case sendTool:
				input.Method = contracts.Send.Name()
			case cancelTool:
				input.Method = contracts.Cancel.Name()
			default:
				return pluginsdk.ToolReceipt{}, nil
			}
			return c.receipt(ctx, input)
		}),
		backend.BindCapability(capabilityExport, contracts.Capability),
	)
}

func commandTool[P any](name string, method frame.MethodDescriptor, execute func(context.Context, string, string, framevalue.JSON) (wire.Result, error)) (tool.Tool, error) {
	return tool.NewFunc(tool.FuncConfig{Name: name, Description: method.Description}, func(ctx context.Context, _ P) (wire.Result, error) {
		id, err := pluginsdk.ToolCommandID(ctx)
		if err != nil {
			return wire.Result{}, err
		}
		input, err := pluginmcp.RequestInput(ctx)
		if err != nil {
			return wire.Result{}, err
		}
		result, err := execute(ctx, method.Name, id, input)
		return result, pluginsdk.RefusedToolFailure(err)
	})
}

func (c *connector) receipt(ctx context.Context, input pluginmcp.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
	receipt, err := pluginstorage.ReadReceipt(ctx, c.db, input)
	if err != nil || !receipt.Found {
		return pluginsdk.ToolReceipt{}, err
	}
	result, err := pluginmcp.DecodeReply[wire.Result](receipt.Reply)
	return pluginsdk.ToolReceiptFrom(result, err, pluginsdk.RefusedToolFailure)
}
