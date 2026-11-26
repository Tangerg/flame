package delivery

import (
	"fmt"
	"iter"
	"log/slog"
	"strconv"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

func presentRunEvent(event runs.ProjectionEvent) protocol.StreamEvent {
	result := protocol.StreamEvent{Type: event.Kind()}
	switch event := event.(type) {
	case runs.SegmentStarted:
		result.Run = new(presentRun(event.Run))
	case runs.SegmentProgressed:
		result.Progress = new(presentProgress(event.Progress))
	case runs.SegmentFinished:
		result.Run = new(presentRun(event.Run))
		result.Interrupts = presentInterrupts(event.Interrupts)
	case runs.ItemStarted:
		result.Item = new(presentItemStart(event.Item))
	case runs.ItemChanged:
		result.ItemID = event.ItemID.String()
		result.Delta = new(presentDelta(event.Delta))
	case runs.ItemCompleted:
		result.Item = new(presentItem(event.Item))
	case runs.QuestionAnswered:
		result.Item = new(presentItem(event.Item))
	case runs.PlanSnapshot:
		result.Plan = new(presentPlan(event))
	default:
		panic("delivery: unknown canonical run event")
	}
	return result
}

func presentPlan(event runs.PlanSnapshot) protocol.Plan {
	state := presentPlanState(event.Revision, event.UpdatedAt, event.Steps)
	return protocol.Plan{SessionID: event.SessionID.String(), State: &state}
}

func presentPlanState(revision uint64, updatedAt time.Time, steps []plan.Step) protocol.PlanState {
	return protocol.PlanState{
		Revision: revision, Steps: presentPlanStepList(steps), UpdatedAt: updatedAt,
	}
}

func presentPlanStepList(steps []plan.Step) []protocol.PlanStep {
	presented := make([]protocol.PlanStep, 0, len(steps))
	for index, step := range steps {
		presented = append(presented, protocol.PlanStep{
			ID: strconv.Itoa(index), Description: step.Description, Status: presentPlanStatus(step.Status),
		})
	}
	return presented
}

func presentStoredPlan(sessionID identity.SessionID, current plan.Current) protocol.Plan {
	out := protocol.Plan{SessionID: sessionID.String()}
	state, committed := current.State()
	if !committed {
		return out
	}
	presented := presentPlanState(state.Revision(), state.UpdatedAt(), state.Steps())
	out.State = &presented
	return out
}

func presentPlanStatus(status plan.Status) protocol.PlanStatus {
	switch status {
	case plan.StatusPending:
		return protocol.PlanStatusPending
	case plan.StatusInProgress:
		return protocol.PlanStatusInProgress
	case plan.StatusCompleted:
		return protocol.PlanStatusCompleted
	default:
		panic("delivery: unknown plan status")
	}
}

func mapRunEvents(in iter.Seq2[runs.Event, error]) iter.Seq2[protocol.RunEvent, error] {
	return func(yield func(protocol.RunEvent, error) bool) {
		for event, observationErr := range in {
			if observationErr != nil {
				yield(protocol.RunEvent{}, wireLiveSegmentError(observationErr))
				return
			}
			presented, err := presentedRunEvent(event.Payload)
			if err != nil {
				yield(protocol.RunEvent{}, err)
				return
			}
			wire := protocol.RunEvent{
				RunID: event.RunID.String(), SegmentID: event.SegmentID.String(),
				EventID: protocol.IDPrefixEvent + event.Cursor, Timestamp: event.Timestamp,
				Event: presented,
			}
			if !yield(wire, nil) {
				return
			}
		}
	}
}

func presentedRunEvent(event runs.ProjectionEvent) (presented protocol.StreamEvent, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			presented = protocol.StreamEvent{}
			slog.Error("delivery: run event presenter panicked",
				telemetry.Panic(recovered))
			err = protocol.NewFailure(protocol.ErrInternalError, fmt.Sprintf("the runtime could not present a run event: %v", recovered))
		}
	}()
	return presentRunEvent(event), nil
}
