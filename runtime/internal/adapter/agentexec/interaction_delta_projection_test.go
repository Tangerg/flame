package agentexec

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Tangerg/scope/agent"
)

// TestUnparsableDeltaIsReportedNotDropped covers the one thing a delta listener
// can still do when it cannot forward an increment: say so. A delta the Runtime
// cannot decode leaves the same hole in a client's preview as one with no
// route, and it additionally means the execution framework handed this Runtime a
// payload outside the contract they share.
func TestUnparsableDeltaIsReportedNotDropped(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	ctx, span := provider.Tracer("test").Start(t.Context(), "project-delta")
	session := &interactionSession{}
	// The zero Delta carries no payload, which the strict Interaction decoder
	// rejects — the same class of failure as an unknown wire member.
	session.projectDelta(ctx, agent.Delta{})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("captured %d spans, want 1", len(spans))
	}
	var cause, exception string
	for _, event := range spans[0].Events {
		for _, attribute := range event.Attributes {
			switch {
			case event.Name == "agentexec.delta.dropped" && attribute.Key == "drop.cause":
				cause = attribute.Value.AsString()
			case event.Name == "exception" && attribute.Key == "exception.message":
				exception = attribute.Value.AsString()
			}
		}
	}
	if !strings.Contains(cause, "decode model response Delta") {
		t.Fatalf("drop cause = %q, want the decoder failure", cause)
	}
	// A payload outside the shared contract is a defect, not the routine drop
	// of an increment whose run tree stopped accepting events.
	if !strings.Contains(exception, "decode model response Delta") {
		t.Fatalf("recorded error = %q, want the decoder failure", exception)
	}
}
