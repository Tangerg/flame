package runs

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func (p Pending) ProjectInterrupts(itemsByID map[identity.ItemID]transcript.Item) ([]transcript.Interrupt, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	projected := make([]transcript.Interrupt, len(p.Interrupts))
	for index, open := range p.Interrupts {
		binding := p.Bindings[index]
		continuation, _ := continuationForMember(p.Continuations, binding.MemberID)
		item, found := itemsByID[open.ItemID]
		if !found || item.ID() != open.ItemID || item.RunID() != continuation.RunID {
			return nil, fmt.Errorf(
				"interrupts: interrupt Item %q is not an Item of Run %q",
				open.ItemID, continuation.RunID,
			)
		}
		value, err := open.project(item)
		if err != nil {
			return nil, fmt.Errorf("interrupts: interrupt Item %q: %w", open.ItemID, err)
		}
		projected[index] = value
	}
	return projected, nil
}

func (o OpenInterrupt) project(item transcript.Item) (transcript.Interrupt, error) {
	projected := transcript.Interrupt{
		ItemID: item.ID(), ItemOccurredAt: item.OccurredAt(),
		RunID: item.RunID(),
	}
	switch o.Type() {
	case run.InterruptApproval:
		call, present := item.ToolCall()
		if !present || item.Status() != transcript.ItemRunning || call.Approval != nil {
			return transcript.Interrupt{}, fmt.Errorf("is not a ToolCall awaiting a verdict")
		}
		projected.Prompt = transcript.Approval{
			Tool: call.Invocation, Risk: o.Approval.Risk,
			Reason: o.Approval.Reason, Rememberable: o.Approval.Rememberable,
		}
	case run.InterruptQuestion:
		question, present := item.Question()
		if !present || question.Answered() {
			return transcript.Interrupt{}, fmt.Errorf("is not a Question awaiting answers")
		}
		projected.Prompt = question
	}
	return projected, nil
}

func OpenInterruptsOf(projected []transcript.Interrupt) []OpenInterrupt {
	open := make([]OpenInterrupt, len(projected))
	for index, value := range projected {
		open[index] = OpenInterrupt{ItemID: value.ItemID}
		if prompt, ok := value.Prompt.(transcript.Approval); ok {
			open[index].Approval = &ApprovalReview{
				Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
			}
		}
	}
	return open
}

func drainedToolItem(
	itemsByID map[identity.ItemID]transcript.Item,
	runID identity.RunID,
	drained DrainedTool,
) (transcript.Item, error) {
	item, found := itemsByID[drained.ItemID]
	_, isToolCall := item.ToolCall()
	if !found || item.RunID() != runID || !isToolCall || item.Status() != transcript.ItemRunning {
		return transcript.Item{}, fmt.Errorf("drained Tool Item %q is not an open ToolCall of Run %q", drained.ItemID, runID)
	}
	return item, nil
}

func pendingItemIDs(pending Pending) []identity.ItemID {
	ids := make([]identity.ItemID, 0, len(pending.Interrupts))
	for _, open := range pending.Interrupts {
		ids = append(ids, open.ItemID)
	}
	for _, continuation := range pending.Continuations {
		for _, drained := range continuation.DrainedTools {
			ids = append(ids, drained.ItemID)
		}
	}
	return ids
}
