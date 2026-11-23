package execution

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/telemetry"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (i *interactionSession) projectDelta(ctx context.Context, delta scoperun.Delta) {
	parsed, err := interaction.ParseDelta(delta.Payload())
	if err != nil {
		i.reportDroppedDelta(ctx, delta, err)
		return
	}
	model, ok := parsed.(interaction.ModelResponseDelta)
	if !ok {
		i.reportDroppedDelta(ctx, delta, errUnroutableDelta)
		return
	}
	response := model.ResponseDelta()
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

var errUnroutableDelta = errors.New("execution: delta has no accepting executor route")

func (i *interactionSession) reportDroppedDelta(ctx context.Context, delta scoperun.Delta, cause error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(
		"execution.delta.dropped",
		trace.WithAttributes(
			telemetry.ProcessIDKey.String(delta.ProcessID().String()),
			attribute.String("drop.cause", cause.Error()),
		),
	)
	if !errors.Is(cause, errUnroutableDelta) {
		span.RecordError(cause)
		slog.ErrorContext(ctx, "execution: model response delta dropped",
			telemetry.ProcessID(delta.ProcessID().String()), telemetry.Error(cause))
	}
}

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
		slog.ErrorContext(i.lifetime.releasing, "execution: observe Interaction log", telemetry.Error(err))
	}
}
