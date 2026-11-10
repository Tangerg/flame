package main

import (
	"context"
	"errors"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"
	framesqlite "github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func registerManagementCommands(backend *pluginmcp.Backend, db *framesqlite.Database, contracts rpc.Contracts, coordinator *app.Coordinator) error {
	return errors.Join(
		pluginstorage.RegisterCommand(backend, db, contracts.Create, classify, func(ctx context.Context, _ *sdk.ServerSession, input schedule.Draft) (pluginstorage.CommandPlan[schedule.Snapshot], error) {
			mutation, err := coordinator.PrepareCreate(ctx, input)
			if err != nil {
				return pluginstorage.CommandPlan[schedule.Snapshot]{}, err
			}
			return pluginstorage.NewCommandPlan(mutation.State().Snapshot(), mutation.Apply)
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Update, classify, func(ctx context.Context, _ *sdk.ServerSession, input wire.UpdateInput) (pluginstorage.CommandPlan[schedule.Snapshot], error) {
			mutation, err := coordinator.PrepareUpdate(ctx, app.UpdateCommand{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Patch: app.Patch{Title: input.Title, Instructions: input.Instructions, CWD: input.CWD, Cron: input.Cron, TimeZone: input.TimeZone, Enabled: input.Enabled, ModelSelection: input.ModelSelection.Patch()}})
			if err != nil {
				return pluginstorage.CommandPlan[schedule.Snapshot]{}, err
			}
			return pluginstorage.NewCommandPlan(mutation.State().Snapshot(), mutation.Apply)
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Delete, classify, func(ctx context.Context, _ *sdk.ServerSession, input wire.ItemInput) (pluginstorage.CommandPlan[wire.Ack], error) {
			apply, err := coordinator.PrepareDelete(ctx, input.ID)
			if err != nil {
				return pluginstorage.CommandPlan[wire.Ack]{}, err
			}
			return pluginstorage.NewCommandPlan(wire.Ack{}, apply)
		}),
	)
}
