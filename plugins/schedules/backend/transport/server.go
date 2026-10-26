package transport

import (
	"context"
	"database/sql"
	"errors"
	"io"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/backend/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/plugins/schedules/backend/rpc"
	"github.com/Tangerg/flame/plugins/schedules/backend/tools"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/google/uuid"
)

type Store interface {
	app.ManagementStore
	app.WorkerStore
	QueueManual(context.Context, schedule.RunRequest) error
	PendingManual(context.Context, string, int) ([]schedule.RunRequest, error)
	AcceptRun(context.Context, string) error
	CapturedRun(context.Context, string) (schedule.RunRequest, error)
}

func RegisterServer(server *sdk.Server, db *sql.DB, store Store, paths app.CWDResolver, models app.ModelAdmitter, connect RunConnector) (*pluginsdk.Backend, error) {
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
	modelTools, err := tools.BuildSchedules(toolCommands{coordinator: coordinator, db: db})
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.RegisterTools(server, modelTools...); err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, db),
		pluginsdk.RegisterServer(backend, pluginsdk.ToolReceiptMethod, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input pluginsdk.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
			return toolReceipt(ctx, db, input)
		}),
		contracts.List.RegisterServer(backend, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.ListInput]) (rpc.Page, error) {
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
		pluginstorage.RegisterCommand(backend, db, contracts.Create, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input schedule.Draft) (schedule.Snapshot, error) {
			value, err := coordinator.Create(ctx, input)
			return value.Snapshot(), err
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Update, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.UpdateInput) (schedule.Snapshot, error) {
			value, err := coordinator.Update(ctx, app.UpdateCommand{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Patch: app.Patch{Title: input.Title, Instructions: input.Instructions, CWD: input.CWD, Cron: input.Cron, Enabled: input.Enabled, ModelSelection: input.ModelSelection.Patch()}})
			return value.Snapshot(), err
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Delete, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ItemInput) (rpc.Ack, error) {
			return rpc.Ack{}, coordinator.Delete(ctx, input.ID)
		}),
		contracts.RunNow.RegisterServer(backend, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.ItemInput]) (app.RunReference, error) {
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
}

func (t toolCommands) ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[schedule.Schedule], error) {
	return t.coordinator.ListPage(ctx, cursor, limit)
}
func (t toolCommands) Create(ctx context.Context, input schedule.Draft) (schedule.Schedule, error) {
	id, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	admitted, err := pluginsdk.RequestInput(ctx)
	if err != nil {
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	value, err := pluginstorage.Command(ctx, t.db, rpc.Create, id, input, admitted, rpc.Classify, func(ctx context.Context, input schedule.Draft) (schedule.Snapshot, error) {
		value, err := t.coordinator.Create(ctx, input)
		return value.Snapshot(), err
	})
	if err != nil {
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	return schedule.Restore(value)
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
	_, err = pluginstorage.Command(ctx, t.db, rpc.Delete, invocation, rpc.ItemInput{ID: id}, admitted, rpc.Classify, func(ctx context.Context, input rpc.ItemInput) (rpc.Ack, error) {
		return rpc.Ack{}, t.coordinator.Delete(ctx, id)
	})
	return rpc.DecodeError(err)
}
