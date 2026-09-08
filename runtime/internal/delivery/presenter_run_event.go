package delivery

import (
	"fmt"
	"iter"
	"strconv"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/protocol"
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
		outcome, metrics := presentSegmentFinished(event.Run, event.Interrupts)
		return protocol.StreamEvent{
			Type: protocol.StreamSegmentFinished, Outcome: &outcome, Metrics: &metrics,
			ContextTokens: new(event.Run.ContextTokens()),
		}
	case runs.ItemStarted:
		item := presentItemStart(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &item}
	case runs.ItemChanged:
		delta := presentDelta(event.Delta)
		return protocol.StreamEvent{Type: protocol.StreamItemDelta, ItemID: event.ItemID, Delta: &delta}
	case runs.ItemCompleted:
		item := presentItem(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}
	case runs.PlanSnapshot:
		plan := presentPlan(event)
		return protocol.StreamEvent{Type: protocol.StreamPlanUpdated, Plan: &plan}
	default:
		panic("delivery: unknown canonical run event")
	}
}

// presentPlan publishes what a root Run changed. The stream and plan.get go through
// one shape, so live following and cold recovery cannot disagree about the Plan.
func presentPlan(event runs.PlanSnapshot) protocol.Plan {
	state := presentPlanState(event.Revision, event.UpdatedAt, event.Steps)
	return protocol.Plan{SessionID: event.SessionID, State: &state}
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

// presentStoredPlan is the same projection read cold. It goes through the run-event
// shape so the two cannot describe the list differently: one presenter, one answer.
func presentStoredPlan(sessionID string, current plan.Current) protocol.Plan {
	out := protocol.Plan{SessionID: sessionID}
	state, committed := current.State()
	if !committed {
		return out
	}
	presented := presentPlanState(state.Revision(), state.UpdatedAt(), state.Steps())
	out.State = &presented
	return out
}

// presentPlanSteps is the list a portable archive carries: the same items as
// the live projection, through the same presenter, with none of the revision or
// timestamp the archive deliberately leaves behind.
func presentPlanSteps(steps []plan.Step) []protocol.PlanStep {
	return presentPlanStepList(steps)
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

// mapRunEvents publishes the application's run events. A presenter defect ends
// the stream with its cause rather than with a clean close: a consumer must be
// able to tell "this run stopped producing events" from "this runtime could not
// describe one".
func mapRunEvents(in iter.Seq[runs.Event]) iter.Seq2[protocol.RunEvent, error] {
	return func(yield func(protocol.RunEvent, error) bool) {
		for event := range in {
			presented, err := presentedRunEvent(event.Payload)
			if err != nil {
				yield(protocol.RunEvent{}, err)
				return
			}
			wire := protocol.RunEvent{
				RunID: event.RunID, SegmentID: event.SegmentID,
				EventID: protocol.IDPrefixEvent + event.Cursor, Timestamp: event.Timestamp,
				Event: presented,
			}
			if !yield(wire, nil) {
				return
			}
		}
	}
}

// presentedRunEvent contains a presenter defect and reports it as the stream's
// failure. It wraps only the presenter call: a panic raised by the downstream
// range body travels through yield and must keep travelling.
func presentedRunEvent(event runs.ProjectionEvent) (presented protocol.StreamEvent, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			presented = protocol.StreamEvent{}
			err = NewFailure(protocol.ErrInternalError, fmt.Sprintf("the runtime could not present a run event: %v", recovered))
		}
	}()
	return presentRunEvent(event), nil
}
