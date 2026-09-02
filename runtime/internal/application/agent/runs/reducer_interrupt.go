package runs

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type interruptProjection struct {
	events        []ProjectionEvent
	items         []transcript.Item
	approvalItems map[int]transcript.Item
}

func (r *reducer) interrupt(e SegmentInterrupted) (factReduction, error) {
	if err := e.validate(); err != nil {
		return factReduction{}, err
	}
	out, err := r.closeStreaming(transcript.MessageCommentary)
	if err != nil {
		return factReduction{}, err
	}
	projection := interruptProjection{
		events: out, items: completedEventItems(nil, out),
		approvalItems: make(map[int]transcript.Item),
	}
	open := r.tools.drain()
	matched, err := matchInterruptTools(open, e.Interrupts)
	if err != nil {
		return factReduction{}, err
	}
	priorDrained := r.resume.remainingDrainedTools()
	r.drained = mergeDrainedTools(priorDrained, drainedToolRefs(open, matched, e.Interrupts))
	if err := r.projectInterruptedTools(&projection, open, matched, e.Interrupts); err != nil {
		return factReduction{}, err
	}
	pending, err := r.projectPendingInterrupts(&projection, e.Interrupts)
	if err != nil {
		return factReduction{}, err
	}

	r.segmentDuration = e.Duration
	waiting, err := r.runRecord(run.Waiting)
	if err != nil {
		return factReduction{}, err
	}
	return factReduction{
		events:          append(projection.events, SegmentFinished{Run: waiting, Interrupts: pending}),
		parkItems:       projection.items,
		toolInvocations: closedToolInvocationCommits(r.cfg.SegmentID, open),
	}, nil
}

func (r *reducer) projectInterruptedTools(
	projection *interruptProjection,
	open []*openTool,
	matched map[*openTool]int,
	interrupts []Interrupt,
) error {
	for _, ref := range open {
		if index, ok := matched[ref]; ok {
			if err := r.projectInterruptedTool(projection, ref, index, interrupts[index]); err != nil {
				return err
			}
			continue
		}
		if ref.end != nil {
			completed, err := r.completeTool(ref, *ref.end)
			if err != nil {
				return err
			}
			projection.events = append(projection.events, completed...)
			projection.items = completedEventItems(projection.items, completed)
			continue
		}
		suspended, err := r.suspendedToolItem(ref)
		if err != nil {
			return err
		}
		projection.items = append(projection.items, suspended)
	}
	return nil
}

func (r *reducer) projectInterruptedTool(
	projection *interruptProjection,
	ref *openTool,
	interruptIndex int,
	pending Interrupt,
) error {
	if err := r.closeSuspendedToolAttempt(ref); err != nil {
		return err
	}
	switch pending.Kind {
	case interrupt.Approval:
		item, publishStart, err := r.approvalItem(*pending.Approval, ref)
		if err != nil {
			return err
		}
		projection.approvalItems[interruptIndex] = item
		return projection.appendToolItem(item, publishStart)
	case interrupt.Question:
		// The Question is a separate completed prompt Item, while its Tool
		// remains suspended inside the handler and must resume under one identity.
		item, err := r.runningToolItem(ref)
		if err != nil {
			return err
		}
		projection.items = append(projection.items, item)
	}
	return nil
}

func (r *reducer) projectPendingInterrupts(
	projection *interruptProjection,
	interrupts []Interrupt,
) ([]transcript.Interrupt, error) {
	pending := make([]transcript.Interrupt, 0, len(interrupts))
	for index, value := range interrupts {
		projected, err := r.projectPendingInterrupt(projection, index, value)
		if err != nil {
			return nil, err
		}
		pending = append(pending, projected)
	}
	return pending, nil
}

func (r *reducer) projectPendingInterrupt(
	projection *interruptProjection,
	index int,
	pending Interrupt,
) (transcript.Interrupt, error) {
	switch pending.Kind {
	case interrupt.Approval:
		if item, ok := projection.approvalItems[index]; ok {
			return approvalTranscriptInterrupt(item, *pending.Approval), nil
		}
		item, projected, publishStart, err := r.approvalInterrupt(pending)
		if err != nil {
			return transcript.Interrupt{}, err
		}
		if err := projection.appendToolItem(item, publishStart); err != nil {
			return transcript.Interrupt{}, err
		}
		return projected, nil
	case interrupt.Question:
		item, projected, err := r.questionInterrupt(pending)
		if err != nil {
			return transcript.Interrupt{}, err
		}
		projection.events = append(projection.events, ItemCompleted{Item: item})
		projection.items = append(projection.items, item)
		return projected, nil
	default:
		return transcript.Interrupt{}, fmt.Errorf("interrupt kind %q is invalid", pending.Kind)
	}
}

func (p *interruptProjection) appendToolItem(item transcript.Item, publishStart bool) error {
	p.items = append(p.items, item)
	if !publishStart {
		return nil
	}
	started, err := newToolItemStart(item)
	if err != nil {
		return err
	}
	p.events = append(p.events, ItemStarted{Item: started})
	return nil
}

// suspend closes this Run's Segment because another Run in the same tree raised
// the human-input barrier. It carries no direct interrupts, which distinguishes
// a suspended sibling from the Run that owns the barrier. Logical Tool Items stay
// running across the barrier while their segment-scoped attempts end incomplete.
func (r *reducer) suspend(duration time.Duration) (factReduction, error) {
	out, err := r.closeStreaming(transcript.MessageCommentary)
	if err != nil {
		return factReduction{}, err
	}
	parkItems := completedEventItems(nil, out)
	open := r.tools.drain()
	r.drained = mergeDrainedTools(
		r.resume.remainingDrainedTools(),
		drainedToolRefs(open, nil, nil),
	)
	for _, ref := range open {
		if ref.end != nil {
			completed, completeToolErr := r.completeTool(ref, *ref.end)
			if completeToolErr != nil {
				return factReduction{}, completeToolErr
			}
			out = append(out, completed...)
			parkItems = completedEventItems(parkItems, completed)
			continue
		}
		suspended, suspendedToolItemErr := r.suspendedToolItem(ref)
		if suspendedToolItemErr != nil {
			return factReduction{}, suspendedToolItemErr
		}
		parkItems = append(parkItems, suspended)
	}
	r.segmentDuration = duration
	waiting, err := r.runRecord(run.Waiting)
	if err != nil {
		return factReduction{}, err
	}
	return factReduction{
		events:          append(out, SegmentFinished{Run: waiting}),
		parkItems:       parkItems,
		toolInvocations: closedToolInvocationCommits(r.cfg.SegmentID, open),
	}, nil
}

func completedEventItems(items []transcript.Item, events []ProjectionEvent) []transcript.Item {
	for _, event := range events {
		if completed, ok := event.(ItemCompleted); ok {
			items = append(items, completed.Item)
		}
	}
	return items
}

func (r *reducer) approvalInterrupt(in Interrupt) (transcript.Item, transcript.Interrupt, bool, error) {
	if in.Approval == nil {
		return transcript.Item{}, transcript.Interrupt{}, false, nil
	}
	item, publishStart, err := r.approvalItem(*in.Approval, nil)
	if err != nil {
		return transcript.Item{}, transcript.Interrupt{}, false, err
	}
	return item, approvalTranscriptInterrupt(item, *in.Approval), publishStart, nil
}

func (r *reducer) approvalItem(prompt ApprovalPrompt, ref *openTool) (transcript.Item, bool, error) {
	arguments, err := parseToolArguments(prompt.Arguments)
	if err != nil {
		return transcript.Item{}, false, fmt.Errorf("approval tool %q arguments: %w", prompt.ToolName, err)
	}
	var id string
	var startedAt time.Time
	publishStart := false
	if ref != nil {
		id, startedAt = ref.id, ref.occurredAt
	} else {
		identity, reused, identityErr := r.reuseOrCreateToolItem(prompt.CallID, prompt.ToolName, arguments)
		if identityErr != nil {
			return transcript.Item{}, false, identityErr
		}
		id, startedAt = identity.id, identity.occurredAt
		publishStart = !reused
		r.removeDrained(id)
	}
	item, err := transcript.NewToolCall(
		r.itemIdentity(id, startedAt),
		*newToolInvocation(prompt.ToolName, arguments, nil),
		prompt.SafetyClass,
	)
	return item, publishStart, err
}

func approvalTranscriptInterrupt(item transcript.Item, prompt ApprovalPrompt) transcript.Interrupt {
	invocation, _ := item.ToolInvocation()
	return transcript.Interrupt{
		ItemID:         item.ID(),
		ItemOccurredAt: item.OccurredAt(),
		RunID:          item.RunID(),
		Kind:           interrupt.Approval,
		Approval: &transcript.Approval{
			Tool: invocation, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		},
	}
}

// matchInterruptTools binds an executor interrupt back to the open tool call
// that raised it. Approval carries a provider call ID; question-producing tools
// are correlated by their canonical name and arguments because their handler
// creates the interrupt below the execution wrapper that owns that ID.
func matchInterruptTools(open []*openTool, values []Interrupt) (map[*openTool]int, error) {
	matched := make(map[*openTool]int)
	for index, value := range values {
		toolName, rawArguments := value.Tool()
		if toolName == "" {
			continue
		}
		arguments, err := parseToolArguments(rawArguments)
		if err != nil {
			return nil, fmt.Errorf("%s interrupt tool %q arguments: %w", value.Kind, toolName, err)
		}
		callID := ""
		switch {
		case value.Approval != nil:
			callID = value.Approval.CallID
		case value.Question != nil:
			callID = value.Question.CallID
		}
		for _, ref := range open {
			if ref.end != nil {
				continue
			}
			if _, used := matched[ref]; used {
				continue
			}
			if callID != "" {
				if ref.callID != callID {
					continue
				}
			} else if ref.name != toolName || argumentIdentity(ref.arguments) != argumentIdentity(arguments) {
				continue
			}
			matched[ref] = index
			break
		}
	}
	return matched, nil
}

func drainedToolRefs(
	open []*openTool,
	matched map[*openTool]int,
	interrupts []Interrupt,
) []DrainedTool {
	var drained []DrainedTool
	for _, ref := range open {
		matchedIndex, matchedInterrupt := matched[ref]
		activeApproval := matchedInterrupt && interrupts[matchedIndex].Kind == interrupt.Approval
		if ref.end == nil && !activeApproval {
			drained = append(drained, DrainedTool{
				ItemID: ref.id, ItemOccurredAt: ref.occurredAt,
				CallID: ref.callID, SourceCallID: ref.sourceCallID,
				Name: ref.name, Arguments: ref.arguments.Canonical(),
			})
		}
	}
	return drained
}

func mergeDrainedTools(groups ...[]DrainedTool) []DrainedTool {
	var merged []DrainedTool
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, tool := range group {
			if _, duplicate := seen[tool.ItemID]; duplicate {
				continue
			}
			seen[tool.ItemID] = struct{}{}
			merged = append(merged, tool)
		}
	}
	return merged
}

func (r *reducer) removeDrained(itemID string) {
	r.drained = slices.DeleteFunc(r.drained, func(tool DrainedTool) bool {
		return tool.ItemID == itemID
	})
}

func (r *reducer) incompleteStartedToolItem(ref *openTool) (ItemCompleted, error) {
	if err := r.closeSuspendedToolAttempt(ref); err != nil {
		return ItemCompleted{}, err
	}
	item, err := r.runningToolItem(ref)
	if err != nil {
		return ItemCompleted{}, err
	}
	item, err = item.AbandonStartedToolCall(nil, ref.attemptStartedAt, ref.finishedAt)
	if err != nil {
		return ItemCompleted{}, err
	}
	return ItemCompleted{Item: item}, nil
}

func (r *reducer) abandonUnstartedToolItem(ref *openTool) (ItemCompleted, error) {
	finishedAt := r.now()
	if finishedAt.Before(ref.occurredAt) {
		return ItemCompleted{}, fmt.Errorf("tool call %q finish time precedes occurrence time", ref.callID)
	}
	item, err := r.runningToolItem(ref)
	if err != nil {
		return ItemCompleted{}, err
	}
	item, err = item.AbandonToolCall(nil, finishedAt)
	if err != nil {
		return ItemCompleted{}, err
	}
	return ItemCompleted{Item: item}, nil
}

func (r *reducer) suspendedToolItem(ref *openTool) (transcript.Item, error) {
	if err := r.closeSuspendedToolAttempt(ref); err != nil {
		return transcript.Item{}, err
	}
	return r.runningToolItem(ref)
}

func (r *reducer) closeSuspendedToolAttempt(ref *openTool) error {
	finishedAt := r.now()
	if finishedAt.Before(ref.attemptStartedAt) {
		return fmt.Errorf("tool call %q finish time precedes start time", ref.callID)
	}
	ref.finishedAt = finishedAt
	return nil
}

func (r *reducer) questionInterrupt(in Interrupt) (transcript.Item, transcript.Interrupt, error) {
	if in.Question == nil {
		return transcript.Item{}, transcript.Interrupt{}, nil
	}
	question := in.Question.question()
	id, err := r.nextItemID()
	if err != nil {
		return transcript.Item{}, transcript.Interrupt{}, err
	}
	item, err := transcript.NewQuestion(r.itemIdentity(id, r.now()), question)
	if err != nil {
		return transcript.Item{}, transcript.Interrupt{}, err
	}
	return item, transcript.Interrupt{
		ItemID: id, ItemOccurredAt: item.OccurredAt(),
		RunID: r.cfg.RunID, Kind: interrupt.Question, Question: &question,
	}, nil
}

// openTools owns both call lookup and publication order for one reducer. Direct
// calls retain their actual registration order as object references; provider-
// attributed calls use their immutable model-call position. No synthetic
// process-local sequence is needed to reconstruct either order.
type openTools struct {
	byCallID map[string]*openTool
	direct   []*openTool
}

func newOpenTools() openTools {
	return openTools{byCallID: make(map[string]*openTool)}
}

func (o *openTools) add(tool *openTool) {
	if o.byCallID == nil {
		o.byCallID = make(map[string]*openTool)
	}
	o.byCallID[tool.callID] = tool
	if tool.modelCallSequence == 0 {
		o.direct = append(o.direct, tool)
	}
}

func (o openTools) get(callID string) (*openTool, bool) {
	tool, ok := o.byCallID[callID]
	return tool, ok
}

func (o openTools) count() int { return len(o.byCallID) }

func (o *openTools) remove(callID string) {
	tool, present := o.byCallID[callID]
	if !present {
		return
	}
	delete(o.byCallID, callID)
	if tool.modelCallSequence > 0 {
		return
	}
	for index, direct := range o.direct {
		if direct != tool {
			continue
		}
		copy(o.direct[index:], o.direct[index+1:])
		o.direct[len(o.direct)-1] = nil
		o.direct = o.direct[:len(o.direct)-1]
		return
	}
}

func (o *openTools) drain() []*openTool {
	ordered := o.ordered()
	clear(o.byCallID)
	clear(o.direct)
	o.direct = nil
	return ordered
}

func (o openTools) ordered() []*openTool {
	attributed := make([]*openTool, 0, len(o.byCallID)-len(o.direct))
	for _, tool := range o.byCallID {
		if tool.modelCallSequence > 0 {
			attributed = append(attributed, tool)
		}
	}
	slices.SortFunc(attributed, func(a, b *openTool) int {
		if byModelCall := cmp.Compare(a.modelCallSequence, b.modelCallSequence); byModelCall != 0 {
			return byModelCall
		}
		return cmp.Compare(a.toolCallIndex, b.toolCallIndex)
	})
	return append(slices.Clone(o.direct), attributed...)
}

func (o openTools) clone() openTools {
	cloned := newOpenTools()
	for _, current := range o.ordered() {
		cloned.add(cloneOpenTool(current))
	}
	return cloned
}

func cloneOpenTool(current *openTool) *openTool {
	if current == nil {
		return nil
	}
	tool := *current
	if current.end == nil {
		return &tool
	}
	end := *current.end
	end.MutatedPaths = slices.Clone(current.end.MutatedPaths)
	if current.end.ModelResult != nil {
		modelResult := *current.end.ModelResult
		end.ModelResult = &modelResult
	}
	if current.end.Result != nil {
		result := *current.end.Result
		end.Result = &result
	}
	if current.end.Offload != nil {
		offload := *current.end.Offload
		end.Offload = &offload
	}
	if current.end.Failure != nil {
		failure := *current.end.Failure
		end.Failure = &failure
	}
	tool.end = &end
	return &tool
}

func (r *reducer) drainTools() ([]ProjectionEvent, error) {
	tools := r.tools.drain()
	if len(tools) == 0 {
		return nil, nil
	}
	var out []ProjectionEvent
	for _, ref := range tools {
		if ref.end != nil {
			completed, err := r.completeTool(ref, *ref.end)
			if err != nil {
				return nil, err
			}
			out = append(out, completed...)
			continue
		}
		incomplete, err := r.incompleteStartedToolItem(ref)
		if err != nil {
			return nil, err
		}
		out = append(out, incomplete)
	}
	return out, nil
}
