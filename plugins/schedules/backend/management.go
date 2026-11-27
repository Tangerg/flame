package main

import (
	"context"
	"errors"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	"github.com/Tangerg/flame/plugins/schedules/internal/schedule"
	"github.com/Tangerg/flame/plugins/schedules/internal/scheduling"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"

	framesqlite "github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func registerManagementCommands(backend *pluginmcp.Backend, db framesqlite.Database, contracts rpc.Contracts, coordinator *scheduling.Coordinator) error {
	return errors.Join(
		pluginstorage.RegisterCommand(backend, db, contracts.Create, classify, func(ctx context.Context, _ *sdk.ServerSession, input schedule.Draft) (pluginstorage.CommandPlan[schedule.Snapshot], error) {
			mutation, err := coordinator.PrepareCreate(ctx, input)
			if err != nil {
				return pluginstorage.CommandPlan[schedule.Snapshot]{}, err
			}
			return pluginstorage.NewCommandPlan(mutation.State().Snapshot(), mutation.Apply)
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Update, classify, func(ctx context.Context, _ *sdk.ServerSession, input wire.UpdateInput) (pluginstorage.CommandPlan[schedule.Snapshot], error) {
			mutation, err := coordinator.PrepareUpdate(ctx, scheduling.UpdateCommand{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Patch: scheduling.Patch{ProjectID: input.ProjectID, Title: input.Title, Instructions: input.Instructions, CWD: input.CWD, Cron: input.Cron, TimeZone: input.TimeZone, Enabled: input.Enabled, ModelSelection: input.ModelSelection.Patch()}})
			if err != nil {
				return pluginstorage.CommandPlan[schedule.Snapshot]{}, err
			}
			return pluginstorage.NewCommandPlan(mutation.State().Snapshot(), mutation.Apply)
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Delete, classify, func(ctx context.Context, _ *sdk.ServerSession, input wire.DeleteInput) (pluginstorage.CommandPlan[wire.DeleteResult], error) {
			return prepareScheduleDeletion(ctx, coordinator, input)
		}),
	)
}

func prepareScheduleDeletion(ctx context.Context, coordinator *scheduling.Coordinator, input wire.DeleteInput) (pluginstorage.CommandPlan[wire.DeleteResult], error) {
	mutation, err := coordinator.PrepareDelete(ctx, input.ID, input.ExpectedRevision)
	if err != nil {
		return pluginstorage.CommandPlan[wire.DeleteResult]{}, err
	}
	deleted := mutation.State()
	return pluginstorage.NewCommandPlan(wire.DeleteResult{ID: deleted.ID(), Revision: deleted.Revision()}, mutation.Apply)
}
