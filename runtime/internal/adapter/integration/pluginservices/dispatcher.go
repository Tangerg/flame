package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"log/slog"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/google/uuid"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

type Dispatcher struct {
	service   *Schedules
	callbacks *pluginsdk.Callbacks
	publish   invalidation.Publish
}

func NewDispatcher(service *Schedules, callbacks *pluginsdk.Callbacks, publish invalidation.Publish) (*Dispatcher, error) {
	if service == nil || callbacks == nil {
		return nil, errors.New("schedule: service and Runtime callbacks are required")
	}
	return &Dispatcher{service: service, callbacks: callbacks, publish: publish}, nil
}
func (d *Dispatcher) dispatch(ctx context.Context, binding plugins.ServiceBinding, session *sdk.ClientSession, snapshot schedule.RunRequestSnapshot) (app.RunReference, error) {
	request, err := schedule.RestoreRunRequest(snapshot)
	if err != nil {
		return app.RunReference{}, err
	}
	current, err := d.service.pool.ServiceSession(ctx, binding.ServerID)
	if err != nil {
		return app.RunReference{}, err
	}
	if current != session {
		return app.RunReference{}, errors.New("schedule: captured backend generation was withdrawn")
	}
	metadata, release, err := d.openRuntime(ctx, binding, session)
	if err != nil {
		return app.RunReference{}, err
	}
	defer release()
	method, err := pluginsdk.Resolve(ctx, session, rpc.Dispatch)
	if err != nil {
		return app.RunReference{}, err
	}
	input, err := jsoninput.Encode(rpc.DispatchInput{RunID: request.RunID()})
	if err != nil {
		return app.RunReference{}, err
	}
	result, err := method.InvokeWithMetadata(ctx, "dispatch:"+request.RunID(), input, metadata)
	d.publish.Notify(invalidation.ForSchedules(request.ScheduleID()))
	if err != nil {
		return app.RunReference{}, rpc.DecodeError(err)
	}
	var reference app.RunReference
	if err := json.Unmarshal(result.JSON(), &reference, json.RejectUnknownMembers(true)); err != nil {
		return app.RunReference{}, err
	}
	if reference.SessionID != request.SessionID() || reference.RunID != request.RunID() {
		return app.RunReference{}, errors.New("schedule: backend acceptance differs from captured request")
	}
	return reference, nil
}
func (d *Dispatcher) RunNow(ctx context.Context, id string) (app.RunReference, error) {
	request, err := invokeResolvedCommand[rpc.ItemInput, schedule.RunRequestSnapshot](ctx, d.service.commands, rpc.PrepareManual, rpc.ItemInput{ID: id}, func(value schedule.RunRequestSnapshot) error { _, err := schedule.RestoreRunRequest(value); return err }, func(ctx context.Context) (plugins.ServiceBinding, error) {
		binding, found, err := plugins.ServiceOwner(ctx, d.service.installations, d.service.commands.selections, plugin.SchedulesService)
		if err == nil && !found {
			err = plugin.ErrUnavailable
		}
		return binding, err
	}, d.openRuntime)
	if err != nil {
		return app.RunReference{}, rpc.DecodeError(err)
	}
	return d.dispatch(ctx, request.binding, request.session, request.value)
}
func (d *Dispatcher) RunWorker(ctx context.Context) {
	d.pass(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.pass(ctx)
		}
	}
}
func (d *Dispatcher) pass(ctx context.Context) {
	binding, session, err := serviceSession(ctx, d.service.installations, d.service.commands.selections, d.service.pool, plugin.SchedulesService)
	if errors.Is(err, errInactive) {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "schedule: backend admission failed", "error", err)
		return
	}
	reply, err := invokeResolvedCommand[rpc.ScanInput, []schedule.RunRequestSnapshot](operation.Bind(ctx, "plugins.automatic", uuid.NewString()), d.service.commands, rpc.Scan, rpc.ScanInput{Now: time.Now()}, nil, func(context.Context) (plugins.ServiceBinding, error) { return binding, nil }, d.openRuntime)
	if err != nil {
		slog.ErrorContext(ctx, "schedule: durable scan failed", "error", err)
		return
	}
	requests := reply.value
	if len(requests) > schedule.MaximumDispatchBatch {
		slog.ErrorContext(ctx, "schedule: backend exceeded dispatch bound")
		return
	}
	for _, request := range requests {
		if ctx.Err() != nil {
			return
		}
		if _, err := d.dispatch(ctx, binding, session, request); err != nil {
			slog.ErrorContext(ctx, "schedule: Run admission failed", "run.id", request.RunID, "error", err)
		}
	}
}

func (d *Dispatcher) openRuntime(ctx context.Context, binding plugins.ServiceBinding, session *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
	actor, err := d.service.pool.SessionActor(ctx, binding.ServerID, session)
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	ref, release, err := d.callbacks.OpenRPC(operation.WithActor(ctx, actor), session, pluginsdk.CallbackLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	metadata, err := pluginsdk.CallbackMetadata(ref)
	if err != nil {
		release()
		return sdk.ParamsBase{}, nil, err
	}
	return metadata, release, nil
}
