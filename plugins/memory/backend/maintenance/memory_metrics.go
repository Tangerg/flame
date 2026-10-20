package maintenance

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const memoryMetricsScope = "scope/flame/memory"

var extractedFactCounter = sync.OnceValue(func() metric.Int64Counter {
	// OpenTelemetry returns usable no-op instruments on construction failure.
	counter, _ := otel.Meter(memoryMetricsScope).Int64Counter("memory.facts.extracted",
		metric.WithDescription("Durable facts the consolidator appended to the daily ledger."))
	return counter
})

var publishedMemoryGenerationCounter = sync.OnceValue(func() metric.Int64Counter {
	counter, _ := otel.Meter(memoryMetricsScope).Int64Counter("memory.generations.published",
		metric.WithDescription("Curated memory generations the fold published (a watermark advance)."))
	return counter
})

func recordExtractedFacts(ctx context.Context, count int) {
	if count > 0 {
		extractedFactCounter().Add(ctx, int64(count))
	}
}

func recordPublishedMemoryGeneration(ctx context.Context) {
	publishedMemoryGenerationCounter().Add(ctx, 1)
}
