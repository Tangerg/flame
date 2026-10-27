package main

import (
	"context"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/sandbox/internal/contract"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

const executeTool = "sandbox_execute"

func commandFailure(err error) error {
	refusal, known := errors.AsType[*portable.Refusal](err)
	if !known {
		return err
	}
	failure, failureErr := tool.NewFailure(tool.FailureConfig{Kind: tool.FailureKindRejected, Output: chat.NewTextToolOutput(refusal.Message), Cause: err})
	return errors.Join(failure, failureErr)
}

func newServer(runner *runner) (*sdk.Server, error) {
	if runner == nil {
		return nil, errors.New("sandbox: execution owner is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.sandbox", Version: "1.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	execute, err := tool.NewFunc(tool.FuncConfig{Name: executeTool, Description: contracts.Execute.Descriptor().Description}, func(ctx context.Context, input rpc.ExecuteInput) (sandbox.Result, error) {
		id, err := pluginsdk.ToolCommandID(ctx)
		if err != nil {
			return sandbox.Result{}, err
		}
		original, err := qimumcp.RequestInput(ctx)
		if err != nil {
			return sandbox.Result{}, err
		}
		result, err := runner.execute(ctx, contracts.Execute.Name(), id, input, original)
		return result, commandFailure(err)
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
		contracts.Execute.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[rpc.ExecuteInput]) (sandbox.Result, error) {
			original, err := command.RequestInput()
			if err != nil {
				return sandbox.Result{}, err
			}
			return runner.execute(ctx, contracts.Execute.Name(), command.ID, command.Input, original)
		}),
		contracts.Limits.RegisterServer(backend, classify, func(context.Context, *sdk.ServerSession, qimumcp.Command[struct{}]) (rpc.Limits, error) {
			return runner.limits(), nil
		}),
		toolReceipts.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
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
			if err != nil {
				failure, known := tool.ClassifyFailure(commandFailure(err))
				if !known {
					return pluginsdk.ToolReceipt{}, err
				}
				return pluginsdk.NewToolReceipt(failure.Output(), true)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				return pluginsdk.ToolReceipt{}, err
			}
			output, err := chat.NewJSONToolOutput(encoded)
			if err != nil {
				return pluginsdk.ToolReceipt{}, err
			}
			return pluginsdk.NewToolReceipt(output, false)
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
