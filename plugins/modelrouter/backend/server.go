package main

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/sqlite"
	routing "github.com/Tangerg/flame/plugins/modelrouter/backend/domain"
	"github.com/Tangerg/flame/plugins/modelrouter/backend/rpc"
	"github.com/Tangerg/flame/runtime/model"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newServer(store *store) (*sdk.Server, error) {
	if store == nil || store.db == nil {
		return nil, errors.New("model router: private store is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	selection, err := pluginsdk.ModelSelectionMethod()
	if err != nil {
		return nil, err
	}
	selectionContract, err := pluginsdk.ModelSelectionContract()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.modelrouter", Version: "1.0.0"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, store.db),
		pluginstorage.RegisterCommand(backend, store.db, contracts.Replace, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ReplaceInput) (routing.Snapshot, error) {
			return store.replace(ctx, input.ExpectedRevision, input.Rules)
		}),
		contracts.Get.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, _ pluginsdk.Command[struct{}]) (routing.Snapshot, error) {
			policy, err := store.read(ctx)
			return policy.Snapshot(), err
		}),
		selection.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ModelSelectionRequest]) (pluginsdk.ModelSelection, error) {
			if err := command.Input.Validate(); err != nil {
				return pluginsdk.ModelSelection{}, err
			}
			proposal := command.Input.Proposed
			proposed, err := model.NewWithReasoningEffort(proposal.Provider, proposal.Model, proposal.ReasoningEffort)
			if err != nil {
				return pluginsdk.ModelSelection{}, err
			}
			policy, err := store.read(ctx)
			if err != nil {
				return pluginsdk.ModelSelection{}, err
			}
			selected, err := policy.Select(command.Input.Context, proposed)
			if err != nil {
				return pluginsdk.ModelSelection{}, err
			}
			return pluginsdk.ModelSelection{Provider: selected.Provider(), Model: selected.Model(), ReasoningEffort: selected.ReasoningEffort()}, nil
		}),
		backend.BindCapability("policy", contracts.Capability),
		backend.BindCapability("selection", selectionContract),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

func classify(err error) string {
	if errors.Is(err, errStoredPolicy) {
		return ""
	}
	return rpc.Classify(err)
}
