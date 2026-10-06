package runs

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
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
		if !found || item.SessionID() != p.SessionID || item.RunID() != continuation.RunID {
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
	case interrupt.Approval:
		invocation, present := item.ToolInvocation()
		if item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemRunning ||
			item.ApprovalDecision() != "" || !present {
			return transcript.Interrupt{}, fmt.Errorf("is not a ToolCall awaiting a verdict")
		}
		projected.Approval = &transcript.Approval{
			Tool: invocation, Risk: o.Approval.Risk,
			Reason: o.Approval.Reason, Rememberable: o.Approval.Rememberable,
		}
	case interrupt.Question:
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
	sessionID, runID string,
	drained DrainedTool,
) (transcript.Item, transcript.ToolInvocation, error) {
	item, found := itemsByID[drained.ItemID]
	invocation, hasInvocation := item.ToolInvocation()
	_, hasFailure := item.Failure()
	if !found || item.SessionID() != sessionID || item.RunID() != runID ||
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

// validateInterrupt checks one projected interrupt an event carries.
func validateInterrupt(request transcript.Interrupt) error {
	if err := resourceid.ValidateItem(request.ItemID); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	if request.ItemOccurredAt.IsZero() {
		return errors.New("item occurrence time is required")
	}
	if err := resourceid.ValidateRun(request.RunID); err != nil {
		return fmt.Errorf("pending interrupt: %w", err)
	}
	switch request.Kind {
	case interrupt.Approval:
		if request.Approval == nil || request.Question != nil {
			return errors.New("approval interrupt requires only an approval payload")
		}
		if err := request.Approval.Validate(); err != nil {
			return err
		}
	case interrupt.Question:
		if request.Question == nil || request.Approval != nil {
			return errors.New("question interrupt requires only a question payload")
		}
		if err := request.Question.Validate(); err != nil {
			return err
		}
		if request.Question.Answered() {
			return errors.New("open question interrupt already carries an accepted answer")
		}
	default:
		return fmt.Errorf("unknown interrupt kind %q", request.Kind)
	}
	return nil
}
