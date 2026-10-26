package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/guardrails/internal/contract"
	guardrail "github.com/Tangerg/flame/plugins/guardrails/internal/domain"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func newServer(store *store) (*sdk.Server, error) {
	if store == nil || store.db == nil {
		return nil, errors.New("guardrails: private store is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	input, err := pluginsdk.ModelInputMethod()
	if err != nil {
		return nil, err
	}
	inputContract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.guardrails", Version: "1.0.0"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, store.db),
		pluginstorage.RegisterCommand(backend, store.db, contracts.Replace, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ReplaceInput) (guardrail.Summary, error) {
			return store.replace(ctx, input.ExpectedRevision, input.Phrases)
		}),
		contracts.Get.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[struct{}]) (guardrail.Summary, error) {
			policy, err := store.read(ctx)
			return policy.Summary(), err
		}),
		input.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ModelInputRequest]) (chat.Request, error) {
			if err := command.Input.Validate(); err != nil {
				return chat.Request{}, err
			}
			policy, err := store.read(ctx)
			if err != nil {
				return chat.Request{}, err
			}
			if err := inspectRequest(ctx, policy, &command.Input.Request); err != nil {
				return chat.Request{}, err
			}
			return *command.Input.Request.Clone(), nil
		}),
		backend.BindCapability("policy", contracts.Capability),
		backend.BindCapability("input", inputContract),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}
