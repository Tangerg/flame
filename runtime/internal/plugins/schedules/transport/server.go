package transport

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/tools"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/google/uuid"
)

type Store interface {
	app.ManagementStore
	app.WorkerStore
	QueueManual(context.Context, schedule.RunRequest) error
	PendingManual(context.Context, string, int) ([]schedule.RunRequest, error)
	AcceptRun(context.Context, string) error
}

func RegisterServer(server *sdk.Server, db *sql.DB, store Store, paths app.CWDResolver, models app.ModelAdmitter) error {
	coordinator, err := app.New(app.Dependencies{Store: store, Paths: paths, Models: models, NewScheduleID: func() string { return schedule.IDPrefix + uuid.NewString() }})
	if err != nil {
		return err
	}
	scanner, err := app.NewScanner(store, store, func() string { return identity.SessionPrefix + uuid.NewString() }, func() string { return identity.RunPrefix + uuid.NewString() })
	if err != nil {
		return err
	}
	modelTools, err := tools.BuildSchedules(toolCommands{coordinator: coordinator, db: db})
	if err != nil {
		return err
	}
	if err := pluginsdk.RegisterTools(server, modelTools...); err != nil {
		return err
	}
	return errors.Join(
		managedmcp.RegisterReceipts(server, db),
		pluginsdk.RegisterServer(server, rpc.List, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ListInput) (rpc.Page, error) {
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
		managedmcp.RegisterCommand(server, db, rpc.Create, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input schedule.Draft) (schedule.Snapshot, error) {
			value, err := coordinator.Create(ctx, input)
			return value.Snapshot(), err
		}),
		managedmcp.RegisterCommand(server, db, rpc.Update, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.UpdateInput) (schedule.Snapshot, error) {
			value, err := coordinator.Update(ctx, app.UpdateCommand{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Patch: app.Patch{Title: input.Title, Instructions: input.Instructions, CWD: input.CWD, Cron: input.Cron, Enabled: input.Enabled, ModelSelection: input.ModelSelection}})
			return value.Snapshot(), err
		}),
		managedmcp.RegisterCommand(server, db, rpc.Delete, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ItemInput) (rpc.Ack, error) {
			return rpc.Ack{}, coordinator.Delete(ctx, input.ID)
		}),
		managedmcp.RegisterCommand(server, db, rpc.PrepareManual, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ItemInput) (schedule.RunRequestSnapshot, error) {
			value, err := store.Get(ctx, input.ID)
			if err != nil {
				return schedule.RunRequestSnapshot{}, err
			}
			request, err := value.ManualRun(identity.SessionPrefix+uuid.NewString(), identity.RunPrefix+uuid.NewString(), time.Now())
			if err != nil {
				return schedule.RunRequestSnapshot{}, err
			}
			err = store.QueueManual(ctx, request)
			return request.Snapshot(), err
		}),
		pluginsdk.RegisterServer(server, rpc.Scan, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ScanInput) ([]schedule.RunRequestSnapshot, error) {
			if input.Now.IsZero() {
				return nil, errors.New("schedule: scan time is required")
			}
			requests, err := scanner.Scan(ctx, input.Now)
			values := make([]schedule.RunRequestSnapshot, len(requests))
			for i, value := range requests {
				values[i] = value.Snapshot()
			}
			return values, err
		}),
		pluginsdk.RegisterServer(server, rpc.Accept, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.AcceptInput) (rpc.Ack, error) {
			return rpc.Ack{}, store.AcceptRun(ctx, input.RunID)
		}),
	)
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
	value, err := managedmcp.Command(ctx, t.db, rpc.Create, id, input, rpc.Classify, func(ctx context.Context, input schedule.Draft) (schedule.Snapshot, error) {
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
	_, err = managedmcp.Command(ctx, t.db, rpc.Delete, invocation, rpc.ItemInput{ID: id}, rpc.Classify, func(ctx context.Context, input rpc.ItemInput) (rpc.Ack, error) {
		return rpc.Ack{}, t.coordinator.Delete(ctx, id)
	})
	return rpc.DecodeError(err)
}
