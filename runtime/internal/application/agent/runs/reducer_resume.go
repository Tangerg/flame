package runs

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type resumeBinding struct {
	callItems map[string]resumableItem
	drained   []DrainedTool
	err       error
}

// resumableItem is an open Tool Item a resumed Segment re-binds. Its
// occurrence and invocation are read from the Item itself.
type resumableItem struct {
	id               string
	occurredAt       time.Time
	invocation       transcript.ToolInvocation
	approvalDecision approval.Decision
}

func resumeBindingFrom(continuation treeContinuation, runID string) *resumeBinding {
	builder := newResumeBindingBuilder(continuation.approvalVerdicts)
	if err := builder.addInterrupts(continuation.interrupts, runID); err != nil {
		return &resumeBinding{err: err}
	}
	if member, found := continuation.forRun(runID); found {
		if err := builder.addTools(continuation, member); err != nil {
			return &resumeBinding{err: err}
		}
	}
	return builder.build()
}

type resumeBindingBuilder struct {
	binding          resumeBinding
	approvalVerdicts map[string]approvalVerdict
}

func newResumeBindingBuilder(verdicts map[string]approvalVerdict) *resumeBindingBuilder {
	return &resumeBindingBuilder{approvalVerdicts: verdicts, binding: resumeBinding{
		callItems: make(map[string]resumableItem),
	}}
}

func (r *resumeBindingBuilder) addItem(
	callID string,
	itemID string,
	occurredAt time.Time,
	invocation transcript.ToolInvocation,
	decision approval.Decision,
) {
	r.binding.callItems[callID] = resumableItem{
		id: itemID, occurredAt: occurredAt, invocation: invocation, approvalDecision: decision,
	}
}

func (r *resumeBindingBuilder) addInterrupts(interrupts []transcript.Interrupt, runID string) error {
	for _, pending := range interrupts {
		if pending.RunID != runID || pending.ItemID == "" {
			continue
		}
		switch pending.Kind {
		case interrupt.Approval:
			if pending.Approval != nil && pending.Approval.Tool.Name != "" {
				verdict, found := r.approvalVerdicts[pending.ItemID]
				if !found {
					return fmt.Errorf("resume Tool approval %q has no accepted resolution", pending.ItemID)
				}
				// Accepting the answer settles the verdict, while the Tool Item
				// stays open until execution finishes or activation is abandoned.
				r.binding.drained = append(r.binding.drained, DrainedTool{
					ItemID: pending.ItemID, CallID: verdict.callID,
				})
				r.addItem(
					verdict.callID,
					pending.ItemID,
					pending.ItemOccurredAt,
					pending.Approval.Tool,
					verdict.decision,
				)
			}
		case interrupt.Question:
			// Question Items are complete prompt facts when the tree parks. Pending
			// owns whether an answer is still outstanding, so resume has no Item
			// lifecycle to settle.
		}
	}
	return nil
}

func (r *resumeBindingBuilder) addTools(continuation treeContinuation, member Continuation) error {
	r.binding.drained = append(r.binding.drained, member.DrainedTools...)
	for _, drained := range member.DrainedTools {
		item, invocation, err := drainedToolItem(continuation.items, member.RunID, drained)
		if err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		r.addItem(drained.CallID, drained.ItemID, item.OccurredAt(), invocation, "")
	}
	return nil
}

func (r *resumeBindingBuilder) build() *resumeBinding {
	binding := &r.binding
	if len(binding.callItems) == 0 {
		return nil
	}
	return binding
}

func (r *reducer) reuseOrCreateToolItem(callID string) (resumableItem, bool, error) {
	if r.resume != nil {
		if item, ok := r.resume.callItems[callID]; ok {
			r.resume.consumeToolCall(callID)
			return item, true, nil
		}
	}
	id, err := r.nextItemID()
	if err != nil {
		return resumableItem{}, false, err
	}
	return resumableItem{id: id, occurredAt: r.now()}, false, nil
}

func (r *resumeBinding) consumeToolCall(callID string) {
	delete(r.callItems, callID)
}

func (r *resumeBinding) remainingDrainedTools() []DrainedTool {
	if r == nil || len(r.drained) == 0 {
		return nil
	}
	out := make([]DrainedTool, 0, len(r.drained))
	for _, tool := range r.drained {
		if _, pending := r.callItems[tool.CallID]; pending {
			out = append(out, tool)
		}
	}
	return out
}
