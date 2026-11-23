package goals

import (
	"context"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const instrumentationName = "github.com/Tangerg/flame/runtime/internal/application/goals"

var driverTracer = otel.Tracer(instrumentationName)

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

var goalRunCounter = sync.OnceValue(func() metric.Int64Counter {
	counter, _ := otel.Meter(instrumentationName).Int64Counter("goal.runs",
		metric.WithDescription("Autonomous Goal Runs, by the Goal status each left (active/complete/blocked/paused)."))
	return counter
})

func recordGoalRun(ctx context.Context, status goal.Status) {
	goalRunCounter().Add(ctx, 1, metric.WithAttributes(attribute.String("goal.status", string(status))))
}
