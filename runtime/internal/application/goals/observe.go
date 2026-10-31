package goals

import (
	"context"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// The autonomous drive is request-detached — without a span per Run and a
// settlement metric it runs blind. The drive's ctx keeps the
// starting request's trace values (taskgroup.Attach → context.WithoutCancel), so
// each Goal-run span nests under the root Goal trace. No-op until
// a TracerProvider / MeterProvider is installed.
const observeScope = "scope/flame/goal"

var driverTracer = otel.Tracer(observeScope)

// runSettlement is the Goal status an autonomous Run left behind — the span
// attribute and metric dimension. An active Goal continues the drive with
// another Run. A drive that settled no Run (canceled, superseded or never
// launched) is not metered.
type runSettlement struct {
	status  goal.Status
	settled bool
}

func settledAs(status goal.Status) runSettlement {
	return runSettlement{status: status, settled: true}
}

func (s runSettlement) continues() bool {
	return s.settled && s.status == goal.StatusActive
}

var loadGoalRuns = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(observeScope).Int64Counter("goal.runs",
		metric.WithDescription("Autonomous Goal Runs, by the Goal status each left (active/complete/blocked/paused)."))
	return counter
})

func recordGoalRun(ctx context.Context, status goal.Status) {
	loadGoalRuns().Add(ctx, 1, metric.WithAttributes(attribute.String("goal.status", string(status))))
}
