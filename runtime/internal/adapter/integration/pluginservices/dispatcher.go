package pluginservices

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type Dispatcher struct {
	service *Schedules
	runs    *runs.Coordinator
	publish invalidation.Publish
}

func NewDispatcher(service *Schedules, coordinator *runs.Coordinator, publish invalidation.Publish) (*Dispatcher, error) {
	if service == nil || coordinator == nil {
		return nil, errors.New("schedule: service and Run coordinator are required")
	}
	return &Dispatcher{service: service, runs: coordinator, publish: publish}, nil
}
func (d *Dispatcher) dispatch(ctx context.Context, binding plugins.ServiceBinding, session *sdk.ClientSession, snapshot schedule.RunRequestSnapshot) (runs.StartResult, error) {
	request, err := schedule.RestoreRunRequest(snapshot)
	if err != nil {
		return runs.StartResult{}, err
	}
	current, err := d.service.pool.ServiceSession(ctx, binding.ServerID)
	if err != nil {
		return runs.StartResult{}, err
	}
	if current != session {
		return runs.StartResult{}, errors.New("schedule: captured backend generation was withdrawn")
	}
	execution := request.Execution()
	startContext, cancel := context.WithCancel(ctx)
	result, err := d.runs.StartAutomation(startContext, runs.AutomationStart{
		Origin:    runs.AutomationOrigin{InstallationID: binding.InstallationID.String(), ReleaseDigest: binding.Digest.String(), ServerName: binding.ServerName.String()},
		SessionID: request.SessionID(), RunID: request.RunID(), Title: execution.Title(), Instructions: execution.Instructions(), CWD: execution.CWD(), ModelSelection: execution.ModelSelection(),
	})
	cancel()
	if err != nil {
		return runs.StartResult{}, err
	}
	_, err = managedmcp.Call[rpc.AcceptInput, rpc.Ack](ctx, session, rpc.Accept, rpc.AcceptInput{RunID: request.RunID()})
	d.publish.Notify(invalidation.ForSchedules(request.ScheduleID()))
	if err != nil {
		return runs.StartResult{}, rpc.DecodeError(err)
	}
	return result, nil
}
func (d *Dispatcher) RunNow(ctx context.Context, id string) (runs.StartResult, error) {
	binding, session, err := serviceSession(ctx, d.service.installations, d.service.pool, plugin.SchedulesService)
	if err != nil {
		return runs.StartResult{}, err
	}
	request, err := managedmcp.CallCommand[rpc.ItemInput, schedule.RunRequestSnapshot](ctx, session, rpc.PrepareManual, operation.Identity(ctx), rpc.ItemInput{ID: id})
	if err != nil {
		return runs.StartResult{}, rpc.DecodeError(err)
	}
	return d.dispatch(ctx, binding, session, request)
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
	binding, session, err := serviceSession(ctx, d.service.installations, d.service.pool, plugin.SchedulesService)
	if errors.Is(err, errInactive) {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "schedule: backend admission failed", "error", err)
		return
	}
	requests, err := managedmcp.Call[rpc.ScanInput, []schedule.RunRequestSnapshot](ctx, session, rpc.Scan, rpc.ScanInput{Now: time.Now()})
	if err != nil {
		slog.ErrorContext(ctx, "schedule: durable scan failed", "error", err)
		return
	}
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
