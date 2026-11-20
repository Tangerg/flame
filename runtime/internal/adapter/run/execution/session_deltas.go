package execution

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// projectDelta forwards the preview text a client renders before the durable
// Item exists. Every reason a delta cannot be forwarded leaves the same hole in
// that preview, so every reason reports it: a payload this Runtime cannot parse
// is a framework boundary defect, not a quiet no-op.
func (i *interactionSession) projectDelta(ctx context.Context, delta scoperun.Delta) {
	parsed, err := interaction.ParseModelResponseDelta(delta.Payload())
	if err != nil {
		i.reportDroppedDelta(ctx, delta, err)
		return
	}
	response := parsed.ResponseDelta()
	for _, part := range response.Parts {
		var payload runs.ExecutionFact
		switch part.Kind {
		case corechat.PartDeltaText, corechat.PartDeltaRefusal:
			payload = runs.MessageDelta{Text: part.Text}
		case corechat.PartDeltaReasoning:
			payload = runs.ReasoningDelta{Text: part.Text}
		default:
			continue
		}
		member, found := i.executorMemberByProcessID(delta.ProcessID())
		if found && i.lifetime.offer(runs.ExecutorEvent{Member: member, Payload: payload}) {
			continue
		}
		i.reportDroppedDelta(ctx, delta, errUnroutableDelta)
	}
}

// errUnroutableDelta names the drop that is not a defect: the process has no
// installed executor route, or the run tree stopped accepting events.
var errUnroutableDelta = errors.New("execution: delta has no accepting executor route")

func (i *interactionSession) reportDroppedDelta(ctx context.Context, delta scoperun.Delta, cause error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(
		"execution.delta.dropped",
		trace.WithAttributes(
			attribute.String("process.id", delta.ProcessID().String()),
			attribute.String("drop.cause", cause.Error()),
		),
	)
	if !errors.Is(cause, errUnroutableDelta) {
		span.RecordError(cause)
		slog.ErrorContext(ctx, "execution: model response delta dropped",
			"process.id", delta.ProcessID().String(), "error", cause)
	}
}

// Delta delivery is a host-owned preview channel. Flush asks its sole reader to
// finish every already-emitted preview before the durable result is published.
func (i *interactionSession) flushDeltas(ctx context.Context) error {
	done := make(chan struct{})
	select {
	case i.deltaFlush <- done:
	case <-ctx.Done():
		return ctx.Err()
	case <-i.lifetime.releasing.Done():
		return errInteractionReleased
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-i.lifetime.releasing.Done():
		return errInteractionReleased
	}
}

func (i *interactionSession) consumeDeltas() {
	ctx := i.lifetime.releasing
	for {
		select {
		case delta := <-i.deltas:
			i.projectDelta(ctx, delta)
		case done := <-i.deltaFlush:
			draining := true
			for draining {
				select {
				case delta := <-i.deltas:
					i.projectDelta(ctx, delta)
				default:
					draining = false
				}
			}
			close(done)
		case <-ctx.Done():
			return
		}
	}
}

func (i *interactionSession) consumeTelemetry() {
	process := i.state.processHandle()
	if process == nil {
		return
	}
	if _, err := i.telemetry.Consume(i.lifetime.releasing, process, 0); err != nil && i.lifetime.releasing.Err() == nil {
		slog.ErrorContext(i.lifetime.releasing, "execution: observe Interaction log", "error", err)
	}
}
