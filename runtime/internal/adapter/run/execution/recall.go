package execution

import (
	"sync"

	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const recalledMemoryTopK = 5

const instrumentationName = "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"

var recallTracer = otel.Tracer(instrumentationName)

var recalledMemoryCounter = sync.OnceValue(func() metric.Int64Counter {
	counter, _ := otel.Meter(instrumentationName).Int64Counter(string(telemetry.MemoryRecalledKey),
		metric.WithDescription("Non-pinned memory items retrieved and injected as a turn's recall block."))
	return counter
})
