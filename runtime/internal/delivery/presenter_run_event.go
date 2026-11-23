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
	switch event := event.(type) {
	case runs.SegmentStarted:
		run := presentRun(event.Run)
		return protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: &run}
	case runs.SegmentProgressed:
		progress := presentProgress(event.Progress)
		return protocol.StreamEvent{Type: protocol.StreamSegmentProgress, Progress: &progress}
	case runs.SegmentFinished:
		run := presentRun(event.Run)
		return protocol.StreamEvent{
			Type: protocol.StreamSegmentFinished, Run: &run, Interrupts: presentInterrupts(event.Interrupts),
		}
	case runs.ItemStarted:
		item := presentItemStart(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &item}
	case runs.ItemChanged:
		delta := presentDelta(event.Delta)
		return protocol.StreamEvent{Type: protocol.StreamItemDelta, ItemID: event.ItemID.String(), Delta: &delta}
	case runs.ItemCompleted:
		item := presentItem(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}
	case runs.QuestionAnswered:
		item := presentItem(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}
	case runs.PlanSnapshot:
		plan := presentPlan(event)
		return protocol.StreamEvent{Type: protocol.StreamPlanUpdated, Plan: &plan}
	default:
		panic("delivery: unknown canonical run event")
	}
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

func mapRunEvents(in iter.Seq[runs.Event]) iter.Seq2[protocol.RunEvent, error] {
	return func(yield func(protocol.RunEvent, error) bool) {
		for event := range in {
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
