package execution

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) consumeTelemetry(ctx context.Context) {
	process := i.state.processHandle()
	if process == nil {
		return
	}
	observing, stop := context.WithCancel(ctx)
	retired := make(chan struct{})
	go func() {
		defer close(retired)
		if err := completion.Wait(observing, i.lifetime.done); err != nil {
			return
		}
		// Scope's terminal stream has a committed end and can finish reading
		// it after executor release. A retired waiting tree has no such end.
		inspection, err := process.Inspect(observing)
		if err != nil {
			if observing.Err() == nil && !errors.Is(err, scoperun.ErrTreeNotFound) {
				slog.ErrorContext(observing, "execution: inspect retired Interaction", telemetry.Error(err))
			}
			stop()
			return
		}
		root, _ := inspection.Process(process.ID())
		if !root.Status.Terminal() {
			stop()
		}
	}()
	defer func() {
		stop()
		<-retired
		if err := i.telemetry.Close(ctx); err != nil {
			slog.ErrorContext(ctx, "execution: close Interaction log observer", telemetry.Error(err))
		}
	}()
	if _, err := i.telemetry.Consume(observing, process, 0); err != nil && observing.Err() == nil {
		slog.ErrorContext(observing, "execution: observe Interaction log", telemetry.Error(err))
	}
}
