package main

import (
	"context"
	"database/sql"
	"errors"
	"io"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/google/uuid"
)

type store interface {
	app.ManagementStore
	app.WorkerStore
	QueueManual(context.Context, schedule.RunRequest) error
	PendingManual(context.Context, string, int) ([]schedule.RunRequest, error)
	AcceptRun(context.Context, string) error
	CapturedRun(context.Context, string) (schedule.RunRequest, error)
}

func registerServer(server *sdk.Server, db *sql.DB, store store, paths app.CWDResolver, models app.ModelAdmitter, connect runConnector) (*pluginsdk.Backend, error) {
	if connect == nil {
		return nil, errors.New("schedules: Runtime Run connector is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return nil, err
	}

	coordinator, err := app.New(app.Dependencies{Store: store, Paths: paths, Models: models, NewScheduleID: func() string { return schedule.IDPrefix + uuid.NewString() }})
	if err != nil {
		return nil, err
	}
	scanner, err := app.NewScanner(store, store, func() string { return identity.SessionPrefix + uuid.NewString() }, func() string { return identity.RunPrefix + uuid.NewString() })
	if err != nil {
		return nil, err
	}
	commands := toolCommands{coordinator: coordinator, db: db, store: store, connect: connect}
	modelTools, err := buildSchedules(commands, commands.RunNow)
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, modelTools...); err != nil {
		return nil, err
	}
	toolReceipts, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, db),
		toolReceipts.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			input := command.Input
			return toolReceipt(ctx, db, input)
		}),
		contracts.List.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.ListInput]) (rpc.Page, error) {
			input := command.Input
			limit := pagination.DefaultLimit()
			if input.Limit != nil {
				var err error
				limit, err = pagination.NewLimit(*input.Limit)
				if err != nil {
					return rpc.Page{}, err
				}
			}
			page, err := coordinator.ListPage(ctx, input.Cursor, limit)
			rows := make([]schedule.Snapshot, len(page.Rows))
			for i, value := range page.Rows {
				rows[i] = value.Snapshot()
			}
			return rpc.Page{Rows: rows, NextCursor: page.NextCursor}, err
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Create, classify, func(ctx context.Context, _ *sdk.ServerSession, input schedule.Draft) (schedule.Snapshot, error) {
			value, err := coordinator.Create(ctx, input)
			return value.Snapshot(), err
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Update, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.UpdateInput) (schedule.Snapshot, error) {
			value, err := coordinator.Update(ctx, app.UpdateCommand{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Patch: app.Patch{Title: input.Title, Instructions: input.Instructions, CWD: input.CWD, Cron: input.Cron, Enabled: input.Enabled, ModelSelection: input.ModelSelection.Patch()}})
			return value.Snapshot(), err
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Delete, classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ItemInput) (rpc.Ack, error) {
			return rpc.Ack{}, coordinator.Delete(ctx, input.ID)
		}),
		contracts.RunNow.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.ItemInput]) (app.RunReference, error) {
			return runNow(ctx, db, store, connect, command)
		}),
	); err != nil {
		return nil, err
	}
	if err := backend.BindCapability(rpc.CapabilityReference, contracts.Capability); err != nil {
		return nil, err
	}
	if err := backend.OnSetup(func(ctx context.Context, setup pluginsdk.Setup) (io.Closer, error) {
		values, err := setup.Contributions()
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			if value.Export != rpc.CapabilityReference || value.Background == nil {
				continue
			}
			ref, err := pluginsdk.CallbackFromContext(ctx)
			if err != nil {
				return nil, err
			}
			host, close, err := connect(ctx, ref)
			if err != nil {
				return nil, err
			}
			namespace, err := host.Namespace(ctx)
			if err == nil {
				err = pluginstorage.CheckRuntimeTarget(ctx, db, namespace)
			}
			if err != nil {
				return nil, errors.Join(err, close())
			}
			return newBackground(ctx, db, store, scanner, host, value.ID, *value.Background, close), nil
		}
		return nil, nil
	}); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return backend, nil
}

type toolCommands struct {
	coordinator *app.Coordinator
	db          *sql.DB
	store       store
	connect     runConnector
}

func (t toolCommands) ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[schedule.Schedule], error) {
	return t.coordinator.ListPage(ctx, cursor, limit)
}
func (t toolCommands) Create(ctx context.Context, input schedule.Draft) (schedule.Schedule, error) {
	id, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	admitted, err := pluginsdk.RequestInput(ctx)
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	value, err := pluginstorage.Command(ctx, t.db, rpc.Create, id, input, admitted, classify, func(ctx context.Context, input schedule.Draft) (schedule.Snapshot, error) {
		value, err := t.coordinator.Create(ctx, input)
		return value.Snapshot(), err
	})
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	return schedule.Restore(value)
}
func (t toolCommands) Update(ctx context.Context, input app.UpdateCommand) (schedule.Schedule, error) {
	id, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	admitted, err := pluginsdk.RequestInput(ctx)
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	value, err := pluginstorage.Command(ctx, t.db, rpc.Update, id, input, admitted, classify, func(ctx context.Context, input app.UpdateCommand) (schedule.Snapshot, error) {
		value, err := t.coordinator.Update(ctx, input)
		return value.Snapshot(), err
	})
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	return schedule.Restore(value)
}

func (t toolCommands) RunNow(ctx context.Context, input rpc.ItemInput) (app.RunReference, error) {
	id, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return app.RunReference{}, err
	}
	admitted, err := pluginsdk.RequestInput(ctx)
	if err != nil {
		return app.RunReference{}, err
	}
	value, err := captureManual(ctx, t.db, t.store, t.connect, manualCommand{id: id, input: input, admitted: admitted})
	return value, decodeError(err)
}

func (t toolCommands) Delete(ctx context.Context, id string) error {
	invocation, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return err
	}
	admitted, err := pluginsdk.RequestInput(ctx)
	if err != nil {
		return err
	}
	_, err = pluginstorage.Command(ctx, t.db, rpc.Delete, invocation, rpc.ItemInput{ID: id}, admitted, classify, func(ctx context.Context, input rpc.ItemInput) (rpc.Ack, error) {
		return rpc.Ack{}, t.coordinator.Delete(ctx, id)
	})
	return decodeError(err)
}
