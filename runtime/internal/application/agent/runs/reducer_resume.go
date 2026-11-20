package runs

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type resumeBinding struct {
	callItems map[string]transcript.Item
	drained   []DrainedTool
	err       error
}

func resumeBindingFrom(continuation treeContinuation, runID identity.RunID) *resumeBinding {
	builder := newResumeBindingBuilder()
	if err := builder.addAnsweredApprovals(continuation, runID); err != nil {
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

func (r *resumeBindingBuilder) addAnsweredApprovals(continuation treeContinuation, runID identity.RunID) error {
	for _, answer := range continuation.answered {
		item := continuation.items[answer.InterruptItemID]
		if answer.ToolCallID == "" || item.RunID() != runID {
			continue
		}
		call, ok := item.ToolCall()
		if !ok || call.Approval == nil {
			return fmt.Errorf("resume Tool approval %q has no accepted resolution", item.ID())
		}
		// Accepting the verdict leaves its Tool Item open until execution settles.
		r.binding.drained = append(r.binding.drained, DrainedTool{ItemID: item.ID(), CallID: answer.ToolCallID})
		r.binding.callItems[answer.ToolCallID] = item
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
			original, _ := item.ToolCall()
			if !original.Invocation.MatchesSource(invocation.Name, invocation.Source) {
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
