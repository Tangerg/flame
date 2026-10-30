package main

import (
	"context"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/a2a/internal/contract"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func newServer(connector *connector) (*sdk.Server, error) {
	if connector == nil {
		return nil, errors.New("a2a: connector is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.a2a", Version: "1.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	agents, err := tool.NewFunc(tool.FuncConfig{Name: "a2a_agents", Description: contracts.Agents.Descriptor().Description}, connector.list)
	if err != nil {
		return nil, err
	}
	status, err := tool.NewFunc(tool.FuncConfig{Name: "a2a_status", Description: contracts.Status.Descriptor().Description}, func(ctx context.Context, input wire.TaskRequest) (wire.Result, error) {
		result, err := connector.status(ctx, input)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return result, errors.Join(err, ctx.Err(), context.Cause(ctx))
		}
		kind := tool.FailureKindFailed
		if _, refused := errors.AsType[*portable.Refusal](err); refused {
			kind = tool.FailureKindRejected
		}
		failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: kind, Output: chat.NewTextToolOutput(err.Error()), Cause: err})
		return result, errors.Join(failure, failureErr)
	})
	if err != nil {
		return nil, err
	}
	send, err := commandTool[wire.SendRequest]("a2a_send", contracts.Send.Descriptor(), connector.send)
	if err != nil {
		return nil, err
	}
	cancel, err := commandTool[wire.TaskRequest]("a2a_cancel", contracts.Cancel.Descriptor(), connector.cancel)
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, agents, send, status, cancel); err != nil {
		return nil, err
	}
	receipts, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, connector.db),
		contracts.Agents.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[struct{}]) (wire.Agents, error) {
			return connector.list(ctx, command.Input)
		}),
		contracts.Send.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[wire.SendRequest]) (wire.Result, error) {
			input, err := command.RequestInput()
			if err != nil {
				return wire.Result{}, err
			}
			return connector.send(ctx, contracts.Send.Name(), command.ID, input)
		}),
		contracts.Status.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[wire.TaskRequest]) (wire.Result, error) {
			return connector.status(ctx, command.Input)
		}),
		contracts.Cancel.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[wire.TaskRequest]) (wire.Result, error) {
			input, err := command.RequestInput()
			if err != nil {
				return wire.Result{}, err
			}
			return connector.cancel(ctx, contracts.Cancel.Name(), command.ID, input)
		}),
		receipts.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			input := command.Input
			switch input.Method {
			case "a2a_send":
				input.Method = contracts.Send.Name()
			case "a2a_cancel":
				input.Method = contracts.Cancel.Name()
			default:
				return pluginsdk.ToolReceipt{}, nil
			}
			return connector.receipt(ctx, input)
		}),
		backend.BindCapability("remote-agents", contracts.Capability),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

func commandTool[P any](name string, method portable.MethodDescriptor, execute func(context.Context, string, string, jsonvalue.Value) (wire.Result, error)) (tool.Tool, error) {
	return tool.NewFunc(tool.FuncConfig{Name: name, Description: method.Description}, func(ctx context.Context, _ P) (wire.Result, error) {
		id, err := pluginsdk.ToolCommandID(ctx)
		if err != nil {
			return wire.Result{}, err
		}
		input, err := qimumcp.RequestInput(ctx)
		if err != nil {
			return wire.Result{}, err
		}
		result, err := execute(ctx, method.Name, id, input)
		return result, commandFailure(err)
	})
}

func commandFailure(err error) error {
	refusal, known := errors.AsType[*portable.Refusal](err)
	if !known {
		return err
	}
	failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindRejected, Output: chat.NewTextToolOutput(refusal.Message), Cause: err})
	return errors.Join(failure, failureErr)
}

func (c *connector) receipt(ctx context.Context, input qimumcp.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
	receipt, err := pluginstorage.ReadReceipt(ctx, c.db, input)
	if err != nil || !receipt.Found {
		return pluginsdk.ToolReceipt{}, err
	}
	result, err := qimumcp.DecodeReply[wire.Result](receipt.Reply)
	if err != nil {
		failure, known := tool.ClassifyFailure(commandFailure(err))
		if !known {
			return pluginsdk.ToolReceipt{}, err
		}
		return pluginsdk.NewToolReceipt(failure.Output(), true)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	output, err := chat.NewJSONToolOutput(data)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	return pluginsdk.NewToolReceipt(output, false)
}
