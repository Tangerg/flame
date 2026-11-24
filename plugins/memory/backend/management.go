package main

import (
	"context"
	"errors"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	"github.com/Tangerg/flame/plugins/memory/internal/coordination"

	framesqlite "github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func registerManagementCommands(backend *pluginmcp.Backend, db framesqlite.Database, contracts rpc.Contracts, review *coordination.Coordinator) error {
	return errors.Join(
		pluginstorage.RegisterCommand(backend, db, contracts.Add, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.AddInput) (pluginstorage.CommandPlan[rpc.AddResult], error) {
			mutation, err := review.PrepareAdd(ctx, input.Scope, coordination.Location{CWD: input.CWD, Project: input.Project}, input.Content)
			if err != nil {
				return pluginstorage.CommandPlan[rpc.AddResult]{}, err
			}
			return pluginstorage.NewCommandPlan(rpc.AddResult{Item: rpc.NewItem(mutation.Item()), Changed: mutation.Changed()}, mutation.Apply)
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Update, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.UpdateInput) (pluginstorage.CommandPlan[rpc.Item], error) {
			mutation, err := review.PrepareUpdate(ctx, input.ID, input.ExpectedRevision, input.Content, input.Pinned)
			if err != nil {
				return pluginstorage.CommandPlan[rpc.Item]{}, err
			}
			return pluginstorage.NewCommandPlan(rpc.NewItem(mutation.Item()), mutation.Apply)
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Delete, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ItemInput) (pluginstorage.CommandPlan[rpc.Ack], error) {
			mutation, err := review.PrepareDelete(ctx, input.ID, input.ExpectedRevision)
			if err != nil {
				return pluginstorage.CommandPlan[rpc.Ack]{}, err
			}
			return pluginstorage.NewCommandPlan(rpc.Ack{}, mutation.Apply)
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Review, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ReviewInput) (pluginstorage.CommandPlan[rpc.Ack], error) {
			mutation, err := review.PrepareReview(ctx, input.ID, input.ExpectedRevision, input.Decision)
			if err != nil {
				return pluginstorage.CommandPlan[rpc.Ack]{}, err
			}
			return pluginstorage.NewCommandPlan(rpc.Ack{}, mutation.Apply)
		}),
	)
}
