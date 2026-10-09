package runs

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type resumeBinding struct {
	callItems map[string]transcript.Item
	drained   []DrainedTool
	err       error
}

func resumeBindingFrom(continuation treeContinuation, runID string) *resumeBinding {
	builder := newResumeBindingBuilder()
	if err := builder.addInterrupts(continuation, runID); err != nil {
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
	binding resumeBinding
}

func newResumeBindingBuilder() *resumeBindingBuilder {
	return &resumeBindingBuilder{binding: resumeBinding{
		callItems: make(map[string]transcript.Item),
	}}
}

func (r *resumeBindingBuilder) addInterrupts(continuation treeContinuation, runID string) error {
	for _, pending := range continuation.interrupts {
		if pending.RunID != runID || pending.ItemID == "" {
			continue
		}
		switch pending.Kind {
		case interrupt.Approval:
			if pending.Approval != nil && pending.Approval.Tool.Name != "" {
				callID, found := continuation.approvalCalls[pending.ItemID]
				item := continuation.items[pending.ItemID]
				if !found || callID == "" || !item.ApprovalDecision().Valid() {
					return fmt.Errorf("resume Tool approval %q has no accepted resolution", pending.ItemID)
				}
				// Accepting the answer settles the verdict, while the Tool Item
				// stays open until execution finishes or activation is abandoned.
				r.binding.drained = append(r.binding.drained, DrainedTool{
					ItemID: pending.ItemID, CallID: callID,
				})
				r.binding.callItems[callID] = item
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
		item, _, err := drainedToolItem(continuation.items, member.RunID, drained)
		if err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		r.binding.callItems[drained.CallID] = item
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

func (r *reducer) reuseOrCreateToolItem(callID string, invocation transcript.ToolInvocation, safetyClass tool.SafetyClass) (transcript.Item, bool, error) {
	if r.resume != nil {
		if item, ok := r.resume.callItems[callID]; ok {
			original, _ := item.ToolInvocation()
			if original.Name != invocation.Name {
				return transcript.Item{}, false, fmt.Errorf("resumed Tool call %q changes its name", callID)
			}
			r.resume.consumeToolCall(callID)
			return item, true, nil
		}
	}
	id, err := r.nextItemID()
	if err != nil {
		return transcript.Item{}, false, err
	}
	item, err := transcript.NewToolCall(r.itemIdentity(id, r.now()), invocation, safetyClass)
	return item, false, err
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
