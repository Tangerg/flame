package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"log/slog"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/google/uuid"
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
func (d *Dispatcher) dispatch(ctx context.Context, binding plugins.ContributionBinding, session *sdk.ClientSession, snapshot schedule.RunRequestSnapshot) (app.RunReference, error) {
	request, err := schedule.RestoreRunRequest(snapshot)
	if err != nil {
		return app.RunReference{}, err
	}
	invocation := request.Invocation()
	if invocation.Target.Scope != "installation" || invocation.Target.Reference != binding.InstallationID.String() {
		return app.RunReference{}, errors.New("schedule: captured caller belongs to another installation")
	}
	ctx = operation.Adopt(ctx, invocation.ID, invocation.Origin, invocation.Parent)
	ctx, err = pluginsdk.WithInvocation(ctx, invocation)
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
	exported, err := pluginsdk.ResolveCapability(ctx, session, binding.Export, d.service.contracts.Capability)
	if err != nil {
		return app.RunReference{}, err
	}
	name, id := rpc.Dispatch, "dispatch:"+request.RunID()
	input, err := jsoninput.Encode(rpc.DispatchInput{RunID: request.RunID()})
	if _, manual := request.ManualRecord(); manual {
		name, id = rpc.RunNow, invocation.ID
		input, err = jsoninput.Encode(rpc.ItemInput{ID: request.ScheduleID()})
	}
	if err != nil {
		return app.RunReference{}, err
	}
	method, err := exported.Method(name)
	if err != nil {
		return app.RunReference{}, err
	}
	result, err := method.InvokeWithMetadata(ctx, id, input, metadata)
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
	reply, err := invokeResolvedCommand[rpc.ItemInput, app.RunReference](ctx, d.service.commands, rpc.RunNow, rpc.ItemInput{ID: id}, func(value app.RunReference) error {
		return errors.Join(resourceid.ValidateSession(value.SessionID), resourceid.ValidateRun(value.RunID))
	}, func(ctx context.Context) (plugins.ContributionBinding, error) {
		binding, found, err := plugins.SelectedContribution(ctx, d.service.installations, d.service.commands.selections, d.service.contracts.Capability)
		if err == nil && !found {
			err = plugin.ErrUnavailable
		}
		return binding, err
	}, d.openRuntime)
	return reply.value, rpc.DecodeError(err)
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
	ctx = operation.Start(ctx, capability.Automatic, "schedules.scan", uuid.NewString())
	binding, session, err := contributionSession(ctx, d.service.installations, d.service.commands.selections, d.service.pool, d.service.contracts.Capability)
	if errors.Is(err, errInactive) {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "schedule: backend admission failed", "error", err)
		return
	}
	reply, err := invokeResolvedCommand[rpc.ScanInput, []schedule.RunRequestSnapshot](ctx, d.service.commands, rpc.Scan, rpc.ScanInput{Now: time.Now()}, nil, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, d.openRuntime)
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

func (d *Dispatcher) openRuntime(ctx context.Context, binding plugins.ContributionBinding, session *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
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
