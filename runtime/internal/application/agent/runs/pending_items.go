package runs

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ProjectInterrupts joins each open interrupt with the Item that owns it, in
// the hand-off's canonical order. It refuses an Item that is not open the way
// the hand-off names it: an approval names a running ToolCall with no verdict,
// a Question names an unanswered Question Item, and every Item belongs to the
// Run whose member its binding answers.
func (p Pending) ProjectInterrupts(itemsByID map[string]transcript.Item) ([]transcript.Interrupt, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	projected := make([]transcript.Interrupt, len(p.Interrupts))
	for index, open := range p.Interrupts {
		binding := p.Bindings[index]
		continuation, _ := continuationForMember(p.Continuations, binding.MemberID)
		item, found := itemsByID[open.ItemID]
		if !found || item.RunID() != continuation.RunID {
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
		RunID: item.RunID(), Kind: o.Kind(),
	}
	switch o.Kind() {
	case run.InterruptApproval:
		invocation, present := item.ToolInvocation()
		if item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemRunning ||
			item.ApprovalDecision() != "" || !present {
			return transcript.Interrupt{}, fmt.Errorf("is not a ToolCall awaiting a verdict")
		}
		projected.Approval = &transcript.Approval{
			Tool: invocation, Risk: o.Approval.Risk,
			Reason: o.Approval.Reason, Rememberable: o.Approval.Rememberable,
		}
	case run.InterruptQuestion:
		question, present := item.Question()
		if item.Kind() != transcript.QuestionItem || item.Status() != transcript.ItemCompleted ||
			!present || question.Answered() {
			return transcript.Interrupt{}, fmt.Errorf("is not a Question awaiting answers")
		}
		projected.Question = &question
	}
	return projected, nil
}

// OpenInterruptsOf is a hand-off's share of projected interrupts: the Items
// they name and, for each approval, the policy's review.
func OpenInterruptsOf(projected []transcript.Interrupt) []OpenInterrupt {
	open := make([]OpenInterrupt, len(projected))
	for index, value := range projected {
		open[index] = OpenInterrupt{ItemID: value.ItemID}
		if value.Approval != nil {
			open[index].Approval = &ApprovalReview{
				Risk: value.Approval.Risk, Reason: value.Approval.Reason,
				Rememberable: value.Approval.Rememberable,
			}
		}
	}
	return open
}

// drainedToolItem resolves the open Tool Item a drained tool re-binds. The
// Item, not the hand-off, owns its occurrence and invocation.
func drainedToolItem(
	itemsByID map[string]transcript.Item,
	runID string,
	drained DrainedTool,
) (transcript.Item, transcript.ToolInvocation, error) {
	item, found := itemsByID[drained.ItemID]
	invocation, hasInvocation := item.ToolInvocation()
	_, hasFailure := item.Failure()
	if !found || item.RunID() != runID ||
		item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemRunning ||
		!hasInvocation || hasFailure {
		return transcript.Item{}, transcript.ToolInvocation{}, fmt.Errorf(
			"drained Tool Item %q is not an open ToolCall of Run %q", drained.ItemID, runID,
		)
	}
	return item, invocation, nil
}

// pendingItemIDs lists every Item a hand-off names: its open interrupts and
// its drained Tools.
func pendingItemIDs(pending Pending) []string {
	ids := make([]string, 0, len(pending.Interrupts))
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
