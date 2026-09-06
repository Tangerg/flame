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

type resumableItem struct {
	id               string
	occurredAt       time.Time
	approvalDecision approval.Decision
}

func resumeBindingFrom(continuation treeContinuation, runID string) *resumeBinding {
	builder := newResumeBindingBuilder(continuation.approvalResolutions)
	if err := builder.addInterrupts(continuation.interrupts, runID); err != nil {
		return &resumeBinding{err: err}
	}
	if member, found := continuation.forRun(runID); found {
		if err := builder.addTools(member); err != nil {
			return &resumeBinding{err: err}
		}
	}
	return builder.build()
}

type resumeBindingBuilder struct {
	binding             resumeBinding
	approvalResolutions map[string]ToolApprovalResolution
}

func newResumeBindingBuilder(resolutions map[string]ToolApprovalResolution) *resumeBindingBuilder {
	return &resumeBindingBuilder{approvalResolutions: resolutions, binding: resumeBinding{
		callItems: make(map[string]resumableItem),
	}}
}

func (r *resumeBindingBuilder) addItem(
	callID string,
	itemID string,
	occurredAt time.Time,
	decision approval.Decision,
) {
	r.binding.callItems[callID] = resumableItem{
		id: itemID, occurredAt: occurredAt, approvalDecision: decision,
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
				resolution, found := r.approvalResolutions[pending.ItemID]
				if !found {
					return fmt.Errorf("resume Tool approval %q has no accepted resolution", pending.ItemID)
				}
				// Accepting the answer settles the verdict, while the Tool Item
				// stays open until execution finishes or activation is abandoned.
				r.binding.drained = append(r.binding.drained, DrainedTool{
					ItemID: pending.ItemID, ItemOccurredAt: pending.ItemOccurredAt,
					CallID: resolution.CallID, Name: pending.Approval.Tool.Name,
					Arguments: pending.Approval.Tool.Arguments.Canonical(),
				})
				r.addItem(
					resolution.CallID,
					pending.ItemID,
					pending.ItemOccurredAt,
					resolution.Decision,
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

func (r *resumeBindingBuilder) addTools(member Continuation) error {
	r.binding.drained = append(r.binding.drained, member.DrainedTools...)
	for _, drained := range member.DrainedTools {
		if _, err := parseToolArguments(drained.Arguments); err != nil {
			return fmt.Errorf("resume drained tool %q arguments: %w", drained.Name, err)
		}
		r.addItem(
			drained.CallID,
			drained.ItemID,
			drained.ItemOccurredAt,
			"",
		)
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

func (r *resumeBinding) approvalDecision(callID string) approval.Decision {
	if r == nil {
		return ""
	}
	return r.callItems[callID].approvalDecision
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
