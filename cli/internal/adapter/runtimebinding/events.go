package runtimebinding

import (
	"fmt"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func projectEvent(value protocol.RunEvent) (conversation.RunEvent, bool, error) {
	projection := runEventProjection{source: value}
	projected, err := projection.project()
	if err != nil || !projected.included {
		return conversation.RunEvent{}, projected.included, err
	}
	return projection.envelope(projected.event), true, nil
}

type runEventProjection struct {
	source protocol.RunEvent
}

type projectedRunEvent struct {
	event    conversation.Event
	included bool
}

func includeRunEvent(event conversation.Event) projectedRunEvent {
	return projectedRunEvent{event: event, included: true}
}

func (r runEventProjection) envelope(event conversation.Event) conversation.RunEvent {
	return conversation.RunEvent{
		EventID: r.source.EventID,
		RunID:   r.source.RunID, SegmentID: r.source.SegmentID,
		At: r.source.Timestamp, Event: event,
	}
}

func (r runEventProjection) project() (projectedRunEvent, error) {
	switch r.source.Event.Type {
	case protocol.StreamSegmentStarted:
		return r.segmentStarted()
	case protocol.StreamItemStarted:
		return r.itemStarted()
	case protocol.StreamItemDelta:
		return r.itemDelta()
	case protocol.StreamItemCompleted:
		return r.itemCompleted()
	case protocol.StreamPlanUpdated:
		return r.planUpdated()
	case protocol.StreamSegmentFinished:
		return r.segmentFinished()
	case protocol.StreamSegmentProgress:
		return r.segmentProgress()
	default:
		return projectedRunEvent{}, fmt.Errorf("event %s has unsupported authoritative type %q", r.source.EventID, r.source.Event.Type)
	}
}

func (r runEventProjection) segmentProgress() (projectedRunEvent, error) {
	value := r.source.Event.Progress
	if value == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.progress has no progress", r.source.EventID)
	}
	progress := conversation.RunProgress{
		Activity: value.Activity,
	}
	if value.Step != nil {
		progress.Step = new(*value.Step)
	}
	if value.ContextTokens != nil {
		progress.ContextTokens = new(*value.ContextTokens)
	}
	if value.Usage != nil {
		usage := projectUsageBreakdown(*value.Usage)
		progress.Usage = &usage
	}
	return includeRunEvent(progress), nil
}

func (r runEventProjection) segmentStarted() (projectedRunEvent, error) {
	if r.source.Event.Run == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.started has no run", r.source.EventID)
	}
	run, err := projectRun(*r.source.Event.Run)
	return includeRunEvent(conversation.SegmentStarted{Run: run}), err
}

func (r runEventProjection) itemStarted() (projectedRunEvent, error) {
	if r.source.Event.Item == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.started has no item", r.source.EventID)
	}
	block, err := projectItem(*r.source.Event.Item)
	return includeRunEvent(conversation.BlockStarted{Block: block}), err
}

func (r runEventProjection) itemCompleted() (projectedRunEvent, error) {
	if r.source.Event.Item == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.completed has no item", r.source.EventID)
	}
	block, err := projectItem(*r.source.Event.Item)
	return includeRunEvent(conversation.BlockCompleted{Block: block}), err
}

func (r runEventProjection) itemDelta() (projectedRunEvent, error) {
	delta := r.source.Event.Delta
	if delta == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.delta has no delta", r.source.EventID)
	}
	switch delta.Type {
	case protocol.DeltaToolArguments:
		return includeRunEvent(conversation.ToolArgumentsDelta{
			BlockID: r.source.Event.ItemID, Text: delta.ArgumentsTextDelta,
		}), nil
	case protocol.DeltaContent:
		return includeRunEvent(conversation.BlockDelta{
			BlockID: r.source.Event.ItemID, Text: delta.Text,
		}), nil
	case protocol.DeltaReasoning, protocol.DeltaToolOutput:
		return includeRunEvent(conversation.BlockDelta{BlockID: r.source.Event.ItemID, Text: delta.Text}), nil
	default:
		return projectedRunEvent{}, fmt.Errorf("event %s: unsupported item delta %q", r.source.EventID, delta.Type)
	}
}

func (r runEventProjection) planUpdated() (projectedRunEvent, error) {
	plan, err := projectPlan(r.source.Event.Plan)
	if err != nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: %w", r.source.EventID, err)
	}
	if plan == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: plan.updated has no committed state", r.source.EventID)
	}
	return includeRunEvent(conversation.PlanChanged{Plan: *plan}), nil
}

func (r runEventProjection) segmentFinished() (projectedRunEvent, error) {
	stream := r.source.Event
	if stream.Outcome == nil || stream.Metrics == nil || stream.ContextTokens == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.finished is incomplete", r.source.EventID)
	}
	usage := projectUsage(*stream.Metrics)
	contextTokens := *stream.ContextTokens
	switch stream.Outcome.Type {
	case protocol.SegmentInterrupt:
		interrupts, err := projectInterrupts(stream.Outcome.Interrupts)
		if err != nil {
			return projectedRunEvent{}, fmt.Errorf("event %s: %w", r.source.EventID, err)
		}
		return includeRunEvent(conversation.RunInterrupted{
			Interrupts: interrupts, Usage: usage, ContextTokens: contextTokens,
		}), nil
	case protocol.SegmentSuspended:
		return includeRunEvent(conversation.RunSuspended{Usage: usage, ContextTokens: contextTokens}), nil
	default:
		return includeRunEvent(conversation.RunFinished{
			// Every segment terminal that reaches here is a run terminal: the two
			// segment-only tags are answered by the cases above.
			Outcome: projectOutcome(
				protocol.RunOutcomeType(stream.Outcome.Type),
				stream.Outcome.Error,
				stream.Outcome.Detail,
			),
			Usage:         usage,
			ContextTokens: contextTokens,
		}), nil
	}
}
