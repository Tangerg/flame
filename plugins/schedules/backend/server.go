package main

import (
	"context"
	"errors"
	"io"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/timezone"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	"github.com/Tangerg/flame/plugins/schedules/internal/schedule"
	"github.com/Tangerg/flame/plugins/schedules/internal/scheduling"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"

	framesqlite "github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/google/uuid"
)

type store interface {
	scheduling.ManagementStore
	scheduling.WorkerStore
	QueueManual(context.Context, schedule.RunRequest) error
	PendingManual(context.Context, identity.RunID, int) ([]schedule.RunRequest, error)
	AcceptRun(context.Context, identity.RunID) error
	CapturedRun(context.Context, identity.RunID) (schedule.RunRequest, error)
}

func registerServer(server *sdk.Server, db framesqlite.Database, store store, paths scheduling.CWDResolver) (*pluginmcp.Backend, error) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}

	coordinator, err := scheduling.New(scheduling.Dependencies{Store: store, Paths: paths, NewScheduleID: func() string { return schedule.IDPrefix + uuid.NewString() }})
	if err != nil {
		return nil, err
	}
	scanner, err := scheduling.NewScanner(store, store, newRunIdentity)
	if err != nil {
		return nil, err
	}
	commands := toolCommands{coordinator: coordinator, db: db, store: store}
	modelTools, err := buildSchedules(commands)
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
	handlers := scheduleServer{db: db, store: store, coordinator: coordinator, scanner: scanner}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, db),
		toolReceipts.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[pluginmcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			return toolReceipt(ctx, db, command.Input)
		}),
		contracts.List.RegisterServer(backend, classify, handlers.list),
		registerManagementCommands(backend, db, contracts, coordinator),
		contracts.RunNow.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.ItemInput]) (scheduling.RunReference, error) {
			return runNow(ctx, db, store, command)
		}),
	); err != nil {
		return nil, err
	}
	if err := backend.BindCapability(wire.CapabilityReference, contracts.Capability); err != nil {
		return nil, err
	}
	if err := backend.OnSetup(handlers.setup); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return backend, nil
}

type scheduleServer struct {
	db          framesqlite.Database
	store       store
	coordinator *scheduling.Coordinator
	scanner     *scheduling.Scanner
}

func (s scheduleServer) list(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[wire.ListInput]) (wire.Page, error) {
	input := command.Input
	limit, err := pagination.ParseLimit(input.Limit)
	if err != nil {
		return wire.Page{}, err
	}
	page, err := s.coordinator.ListPage(ctx, input.Cursor, limit)
	rows := make([]schedule.Snapshot, len(page.Rows))
	for i, value := range page.Rows {
		rows[i] = value.Snapshot()
	}
	return wire.Page{Rows: rows, NextCursor: page.NextCursor}, err
}

func (s scheduleServer) setup(ctx context.Context, setup pluginmcp.Setup) (io.Closer, error) {
	values := setup.Contributions()
	for _, value := range values {
		if value.Export != wire.CapabilityReference || value.Background == nil {
			continue
		}
		ref, err := pluginmcp.CallbackFromContext(ctx)
		if err != nil {
			return nil, err
		}
		host, release, err := connectRunHost(ctx, ref)
		if err != nil {
			return nil, err
		}
		namespace, err := host.Namespace(ctx)
		if err == nil {
			err = pluginstorage.CheckRuntimeTarget(ctx, s.db, namespace)
		}
		if err != nil {
			return nil, errors.Join(err, release())
		}
		background, err := newBackground(ctx, s.db, s.store, s.scanner, host, value.ID, *value.Background, release)
		if err != nil {
			return nil, errors.Join(err, release())
		}
		return background, nil
	}
	return nil, nil
}

type toolCommands struct {
	coordinator *scheduling.Coordinator
	db          framesqlite.Database
	store       store
}

func (t toolCommands) ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[schedule.Schedule], error) {
	return t.coordinator.ListPage(ctx, cursor, limit)
}

func (t toolCommands) Create(ctx context.Context, _ createScheduleArgs) (schedule.Schedule, error) {
	id, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	admitted, err := pluginmcp.RequestInput(ctx)
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	value, err := pluginstorage.Command(ctx, t.db, wire.Create, id, admitted, classify, func(ctx context.Context, input createScheduleArgs) (pluginstorage.CommandPlan[schedule.Snapshot], error) {
		draft, err := input.draft(timezone.Host)
		if err != nil {
			return pluginstorage.CommandPlan[schedule.Snapshot]{}, err
		}
		mutation, err := t.coordinator.PrepareCreate(ctx, draft)
		if err != nil {
			return pluginstorage.CommandPlan[schedule.Snapshot]{}, err
		}
		return pluginstorage.NewCommandPlan(mutation.State().Snapshot(), mutation.Apply)
	})
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	return schedule.Restore(value)
}

func (t toolCommands) Update(ctx context.Context, _ updateScheduleArgs) (schedule.Schedule, error) {
	id, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	admitted, err := pluginmcp.RequestInput(ctx)
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	value, err := pluginstorage.Command(ctx, t.db, wire.Update, id, admitted, classify, func(ctx context.Context, input updateScheduleArgs) (pluginstorage.CommandPlan[schedule.Snapshot], error) {
		mutation, err := t.coordinator.PrepareUpdate(ctx, input.command())
		if err != nil {
			return pluginstorage.CommandPlan[schedule.Snapshot]{}, err
		}
		return pluginstorage.NewCommandPlan(mutation.State().Snapshot(), mutation.Apply)
	})
	if err != nil {
		return schedule.Schedule{}, decodeError(err)
	}
	return schedule.Restore(value)
}

func (t toolCommands) RunNow(ctx context.Context, _ wire.ItemInput) (scheduling.RunReference, error) {
	id, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return scheduling.RunReference{}, err
	}
	admitted, err := pluginmcp.RequestInput(ctx)
	if err != nil {
		return scheduling.RunReference{}, err
	}
	value, err := captureManual(ctx, t.db, t.store, manualCommand{id: id, admitted: admitted})
	return value, decodeError(err)
}

func (t toolCommands) Delete(ctx context.Context, _ deleteScheduleArgs) error {
	invocation, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return err
	}
	admitted, err := pluginmcp.RequestInput(ctx)
	if err != nil {
		return err
	}
	_, err = pluginstorage.Command(ctx, t.db, wire.Delete, invocation, admitted, classify, func(ctx context.Context, input deleteScheduleArgs) (pluginstorage.CommandPlan[wire.Ack], error) {
		apply, err := t.coordinator.PrepareDelete(ctx, input.ScheduleID)
		if err != nil {
			return pluginstorage.CommandPlan[wire.Ack]{}, err
		}
		return pluginstorage.NewCommandPlan(wire.Ack{}, apply)
	})
	return decodeError(err)
}

func newRunIdentity() (identity.SessionID, identity.RunID, error) {
	sessionID, err := identity.NewSession(uuid.NewString())
	if err != nil {
		return identity.SessionID{}, identity.RunID{}, err
	}
	runID, err := identity.NewRun(uuid.NewString())
	return sessionID, runID, err
}
