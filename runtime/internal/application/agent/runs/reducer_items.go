package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

const applyingUserInputActivity = "Applying user input"

// openText owns the identity and accumulated content of one live streaming
// item. Reducer clones are speculative, so cloning must also detach the
// mutable builder before a persistence batch can be attempted.
type openText struct {
	identity transcript.ItemIdentity
	content  strings.Builder
}

func newOpenText(identity transcript.ItemIdentity) *openText {
	return &openText{identity: identity}
}

func (o *openText) clone() *openText {
	if o == nil {
		return nil
	}
	cloned := newOpenText(o.identity)
	cloned.append(o.text())
	return cloned
}

func (o *openText) append(text string) { o.content.WriteString(text) }

func (o *openText) itemIdentity() transcript.ItemIdentity { return o.identity }

func (o *openText) itemID() string { return o.identity.ItemID }

func (o *openText) text() string { return o.content.String() }

func (r *reducer) itemIdentity(id string, occurredAt time.Time) transcript.ItemIdentity {
	return transcript.ItemIdentity{
		SessionID:  r.cfg.SessionID,
		RunID:      r.cfg.RunID,
		ItemID:     id,
		OccurredAt: occurredAt,
	}
}

func (r *reducer) appendText(text string) ([]ProjectionEvent, error) {
	if text == "" {
		return nil, nil
	}
	delta, err := newContentItemDelta(text)
	if err != nil {
		return nil, err
	}
	var out []ProjectionEvent
	if r.text == nil {
		id, identityErr := r.nextItemID()
		if identityErr != nil {
			return nil, identityErr
		}
		r.text = newOpenText(r.itemIdentity(id, r.now()))
		start, err := newTransientItemStart(r.text.itemIdentity(), transcript.AgentMessage)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemStarted{Item: start})
	}
	r.text.append(text)
	return append(out, ItemChanged{
		ItemID: r.text.itemID(),
		Delta:  delta,
	}), nil
}

func (r *reducer) appendReasoning(text string) ([]ProjectionEvent, error) {
	if text == "" {
		return nil, nil
	}
	delta, err := newReasoningItemDelta(text)
	if err != nil {
		return nil, err
	}
	var out []ProjectionEvent
	if r.reasoning == nil {
		id, identityErr := r.nextItemID()
		if identityErr != nil {
			return nil, identityErr
		}
		r.reasoning = newOpenText(r.itemIdentity(id, r.now()))
		start, err := newTransientItemStart(r.reasoning.itemIdentity(), transcript.Reasoning)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemStarted{Item: start})
	}
	r.reasoning.append(text)
	return append(out, ItemChanged{
		ItemID: r.reasoning.itemID(),
		Delta:  delta,
	}), nil
}

func (r *reducer) closeText(phase transcript.MessagePhase) ([]ProjectionEvent, error) {
	if r.text == nil {
		return nil, nil
	}
	item, err := transcript.NewAgentMessage(
		r.text.itemIdentity(),
		phase,
		[]transcript.ContentBlock{{Kind: transcript.TextContent, Text: r.text.text()}},
	)
	if err != nil {
		return nil, err
	}
	r.text = nil
	return []ProjectionEvent{ItemCompleted{Item: item}}, nil
}

func (r *reducer) closeReasoning() ([]ProjectionEvent, error) {
	if r.reasoning == nil {
		return nil, nil
	}
	item, err := transcript.NewReasoning(
		r.reasoning.itemIdentity(),
		r.reasoning.text(),
		false,
	)
	if err != nil {
		return nil, err
	}
	r.reasoning = nil
	return []ProjectionEvent{ItemCompleted{Item: item}}, nil
}

func (r *reducer) closeStreaming(phase transcript.MessagePhase) ([]ProjectionEvent, error) {
	reasoning, err := r.closeReasoning()
	if err != nil {
		return nil, err
	}
	message, err := r.closeText(phase)
	if err != nil {
		return nil, err
	}
	return append(reasoning, message...), nil
}

func (r *reducer) completeAssistantMessage(
	message corechat.Message,
	phase transcript.MessagePhase,
) ([]ProjectionEvent, error) {
	if message.Role != corechat.RoleAssistant {
		return nil, fmt.Errorf("completed message role is %q, want %q", message.Role, corechat.RoleAssistant)
	}
	if err := message.Validate(); err != nil {
		return nil, err
	}
	if err := conversation.ValidateMessageIdentities(message); err != nil {
		return nil, err
	}

	var reasoning strings.Builder
	content := make([]transcript.ContentBlock, 0, len(message.Parts))
	for index, part := range message.Parts {
		switch part.Kind {
		case corechat.PartText:
			content = append(content, transcript.ContentBlock{Kind: transcript.TextContent, Text: part.Text})
		case corechat.PartReasoning:
			reasoning.WriteString(part.Text)
		case corechat.PartMedia:
			block, err := assistantMediaBlock(part.Media)
			if err != nil {
				return nil, fmt.Errorf("part[%d]: %w", index, err)
			}
			content = append(content, block)
		case corechat.PartToolCall:
			return nil, fmt.Errorf("part[%d]: completed assistant message still contains a tool call", index)
		default:
			return nil, fmt.Errorf("part[%d]: unsupported assistant part kind %q", index, part.Kind)
		}
	}

	out, err := r.completeReasoning(reasoning.String())
	if err != nil {
		return nil, err
	}
	messageEvents, err := r.completeMessageContent(content, phase)
	if err != nil {
		return nil, err
	}
	out = append(out, messageEvents...)
	return out, nil
}

func (r *reducer) completeModelMessage(
	message corechat.Message,
	phase transcript.MessagePhase,
) ([]ProjectionEvent, error) {
	if message.Role != corechat.RoleAssistant {
		return nil, fmt.Errorf("completed model message role is %q, want %q", message.Role, corechat.RoleAssistant)
	}
	if err := message.Validate(); err != nil {
		return nil, err
	}
	if err := conversation.ValidateMessageIdentities(message); err != nil {
		return nil, err
	}
	semantic := corechat.Message{Role: message.Role}
	for _, part := range message.Parts {
		if part.Kind != corechat.PartToolCall {
			semantic.Parts = append(semantic.Parts, part.Clone())
		}
	}
	if len(semantic.Parts) == 0 {
		return r.closeStreaming(phase)
	}
	return r.completeAssistantMessage(semantic, phase)
}

func messageRequestsToolCalls(message corechat.Message) bool {
	return slices.ContainsFunc(message.Parts, func(part corechat.Part) bool {
		return part.Kind == corechat.PartToolCall
	})
}

func assistantMediaBlock(value *media.Media) (transcript.ContentBlock, error) {
	if value == nil {
		return transcript.ContentBlock{}, errors.New("assistant media is nil")
	}
	if !strings.HasPrefix(value.MIME, "image/") {
		return transcript.ContentBlock{}, fmt.Errorf("assistant media type %q is not supported by Transcript", value.MIME)
	}
	data, err := value.Bytes()
	if err != nil {
		return transcript.ContentBlock{}, fmt.Errorf("assistant image must use an inline byte source: %w", err)
	}
	return transcript.ContentBlock{Kind: transcript.ImageContent, MediaType: value.MIME, Bytes: data}, nil
}

func (r *reducer) completeReasoning(text string) ([]ProjectionEvent, error) {
	if text == "" {
		return r.closeReasoning()
	}
	var identity transcript.ItemIdentity
	started := r.reasoning == nil
	if r.reasoning != nil {
		identity = r.reasoning.itemIdentity()
	} else {
		id, err := r.nextItemID()
		if err != nil {
			return nil, err
		}
		identity = r.itemIdentity(id, r.now())
	}
	r.reasoning = nil
	out := make([]ProjectionEvent, 0, 2)
	if started {
		start, err := newTransientItemStart(identity, transcript.Reasoning)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemStarted{Item: start})
	}
	item, err := transcript.NewReasoning(identity, text, false)
	if err != nil {
		return nil, err
	}
	out = append(out, ItemCompleted{Item: item})
	return out, nil
}

func (r *reducer) completeMessageContent(
	content []transcript.ContentBlock,
	phase transcript.MessagePhase,
) ([]ProjectionEvent, error) {
	if len(content) == 0 {
		return r.closeText(phase)
	}
	var identity transcript.ItemIdentity
	started := r.text == nil
	if r.text != nil {
		identity = r.text.itemIdentity()
	} else {
		id, err := r.nextItemID()
		if err != nil {
			return nil, err
		}
		identity = r.itemIdentity(id, r.now())
	}
	r.text = nil
	out := make([]ProjectionEvent, 0, 2)
	if started {
		start, err := newTransientItemStart(identity, transcript.AgentMessage)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemStarted{Item: start})
	}
	item, err := transcript.NewAgentMessage(identity, phase, content)
	if err != nil {
		return nil, err
	}
	out = append(out, ItemCompleted{Item: item})
	return out, nil
}

func (r *reducer) toolStart(e ToolCallStarted) ([]ProjectionEvent, error) {
	if _, err := runtimeidentity.ParseEffect(e.CallID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(e.ToolName) == "" || e.ToolName != strings.TrimSpace(e.ToolName) {
		return nil, errors.New("tool name is required")
	}
	_, sourceCallPresent, err := conversation.ParseOptionalToolCallIdentity(e.SourceCallID)
	if err != nil {
		return nil, err
	}
	if e.Activity != strings.TrimSpace(e.Activity) {
		return nil, errors.New("tool activity has surrounding whitespace")
	}
	if e.ModelCallSequence == 0 && e.ToolCallIndex != 0 {
		return nil, errors.New("tool call index requires a model call sequence")
	}
	if e.ModelCallSequence > 0 && !sourceCallPresent {
		return nil, errors.New("model-attributed tool call requires a source call id")
	}
	if _, duplicate := r.toolCallIDs[e.CallID]; duplicate {
		return nil, fmt.Errorf("tool call %q started more than once", e.CallID)
	}
	if e.ModelCallSequence > 0 {
		position := toolPosition{
			modelCallSequence: e.ModelCallSequence,
			toolCallIndex:     e.ToolCallIndex,
		}
		if existing, duplicate := r.toolPositions[position]; duplicate {
			return nil, fmt.Errorf(
				"tool call %q repeats model call %d ToolCall index %d already owned by %q",
				e.CallID,
				e.ModelCallSequence,
				e.ToolCallIndex,
				existing,
			)
		}
	}
	arguments, err := parseToolArguments(e.Arguments)
	if err != nil {
		return nil, fmt.Errorf("tool %q arguments: %w", e.ToolName, err)
	}
	if rejectCommittedToolStartErr := r.resume.rejectCommittedToolStart(e.CallID, e.ToolName, arguments); rejectCommittedToolStartErr != nil {
		return nil, rejectCommittedToolStartErr
	}
	out, err := r.closeStreaming(transcript.MessageCommentary)
	if err != nil {
		return nil, err
	}
	// The step number previews the Run's accounting, so it counts the same thing
	// the committed metrics do. Reporting the segment's own count would make a
	// resumed Run appear to start over at step 1.
	metrics, err := r.metrics()
	if err != nil {
		return nil, err
	}
	step := metrics.Steps()
	out = append(out, SegmentProgressed{Progress: Progress{
		Step: &step, Activity: e.Activity,
	}})
	identity, reused, err := r.reuseOrCreateToolItem(e.CallID, e.ToolName, arguments)
	if err != nil {
		return nil, err
	}
	ref := &openTool{
		callID: e.CallID, sourceCallID: e.SourceCallID,
		modelCallSequence: e.ModelCallSequence, toolCallIndex: e.ToolCallIndex,
		id: identity.id, occurredAt: identity.occurredAt, attemptStartedAt: r.now(),
		name: e.ToolName, arguments: arguments, safetyClass: e.SafetyClass,
		approvalDecision: identity.approvalDecision,
	}
	r.toolCallIDs[e.CallID] = struct{}{}
	if e.ModelCallSequence > 0 {
		r.toolPositions[toolPosition{
			modelCallSequence: e.ModelCallSequence,
			toolCallIndex:     e.ToolCallIndex,
		}] = e.CallID
	}
	r.tools.add(ref)
	running, err := r.runningToolItem(ref)
	if err != nil {
		return nil, err
	}
	if !reused {
		start, err := newToolItemStart(running)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemStarted{Item: start})
	}
	if e.Arguments != "" {
		delta, err := newToolArgumentsItemDelta(e.Arguments)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemChanged{
			ItemID: ref.id,
			Delta:  delta,
		})
	}
	return out, nil
}

func (r *reducer) runningToolItem(ref *openTool) (transcript.Item, error) {
	item, err := transcript.NewToolCall(
		r.itemIdentity(ref.id, ref.occurredAt),
		*newToolInvocation(ref.name, ref.arguments, nil),
		ref.safetyClass,
	)
	if err != nil || ref.approvalDecision == "" {
		return item, err
	}
	return item.ResolveToolApproval(ref.approvalDecision)
}

func (r *reducer) openToolItemID(callID string) (string, bool) {
	ref, open := r.tools.get(callID)
	if !open || ref == nil {
		return "", false
	}
	return ref.id, true
}

// spawningItem resolves the executor's immutable parent-call identity to the
// canonical running Item that represents it. Only currently open calls are
// eligible: an AgentTool creates its child before that parent call can finish.
// It returns the complete canonical Item because child admission must persist
// that Item in the same transaction as the child's lineage edge. Ambiguity is
// rejected rather than resolved by ordering.
func (r *reducer) spawningItem(sourceCallID string) (transcript.Item, error) {
	if _, err := conversation.NewToolCallIdentity(sourceCallID); err != nil {
		return transcript.Item{}, err
	}
	var match *openTool
	for _, candidate := range r.tools.byCallID {
		if candidate.sourceCallID != sourceCallID {
			continue
		}
		if match != nil {
			return transcript.Item{}, fmt.Errorf("source call %q identifies multiple open tool items", sourceCallID)
		}
		match = candidate
	}
	if match == nil {
		return transcript.Item{}, fmt.Errorf("source call %q has no open tool item", sourceCallID)
	}
	return r.runningToolItem(match)
}

func (r *reducer) toolEnd(e ToolCallFinished) ([]ProjectionEvent, []ToolInvocationCommit, []corechat.Message, error) {
	if _, err := runtimeidentity.ParseEffect(e.CallID); err != nil {
		return nil, nil, nil, err
	}
	ref, ok := r.tools.get(e.CallID)
	if !ok {
		if consumed, err := r.resume.consumeCommittedTool(e); consumed {
			return nil, nil, nil, err
		}
		return nil, nil, nil, fmt.Errorf("tool call %q ended without an open start", e.CallID)
	}
	if ref.end != nil {
		return nil, nil, nil, fmt.Errorf("tool call %q ended more than once", e.CallID)
	}
	cloned := e
	if e.Offload != nil {
		ref := *e.Offload
		cloned.Offload = &ref
	}
	if e.Failure != nil {
		failure := *e.Failure
		cloned.Failure = &failure
	}
	if e.ModelResult != nil {
		if err := e.ModelResult.Validate(); err != nil {
			return nil, nil, nil, fmt.Errorf("tool call %q has invalid model result: %w", e.CallID, err)
		}
		if e.ModelResult.ID != ref.sourceCallID || e.ModelResult.Name != ref.name {
			return nil, nil, nil, fmt.Errorf("tool call %q model result differs from its source call", e.CallID)
		}
		modelResult := *e.ModelResult
		cloned.ModelResult = &modelResult
	}
	cloned.MutatedPaths = slices.Clone(e.MutatedPaths)
	finishedAt := r.now()
	if finishedAt.Before(ref.attemptStartedAt) {
		return nil, nil, nil, fmt.Errorf("tool call %q finish time precedes start time", e.CallID)
	}
	ref.finishedAt = finishedAt
	ref.end = &cloned
	return r.flushEndedTools()
}

// flushEndedTools commits only the longest completed prefix. Tools may finish
// concurrently in any order, but transcript identity, mutation nudges, and
// durable insertion order must follow the model's call order.
func (r *reducer) flushEndedTools() ([]ProjectionEvent, []ToolInvocationCommit, []corechat.Message, error) {
	ordered := r.tools.ordered()
	var out []ProjectionEvent
	var invocations []ToolInvocationCommit
	var results []corechat.ToolResult
	for _, ref := range ordered {
		if ref.end == nil {
			break
		}
		r.tools.remove(ref.callID)
		completed, err := r.completeTool(ref, *ref.end)
		if err != nil {
			return nil, nil, nil, err
		}
		out = append(out, completed...)
		if r.cfg.Lineage.IsRoot() && ref.modelCallSequence > 0 {
			results = append(results, conversationToolResult(ref, *ref.end))
		}
		if ref.modelCallSequence > 0 {
			invocations = append(invocations, ToolInvocationCommit{
				CallID: ref.callID, ItemID: ref.id, SegmentID: r.cfg.SegmentID,
				State: ToolInvocationCompleted, StartedAt: ref.attemptStartedAt, FinishedAt: ref.finishedAt,
			})
		}
	}
	if len(results) == 0 {
		return out, invocations, nil, nil
	}
	return out, invocations, []corechat.Message{corechat.NewToolMessage(results...)}, nil
}

func conversationToolResult(ref *openTool, finished ToolCallFinished) corechat.ToolResult {
	if finished.ModelResult != nil {
		return *finished.ModelResult
	}
	result := ""
	if finished.Result != nil {
		if text, ok := finished.Result.String(); ok {
			result = text
		} else {
			result = finished.Result.Canonical()
		}
	}
	isError := finished.Failure != nil && finished.Failure.Kind != tool.FailureDenied
	if isError && result == "" {
		result = fmt.Sprintf("error: tool %q failed: %s", ref.name, finished.Failure.Detail)
	}
	return corechat.ToolResult{
		ID: ref.sourceCallID, Name: ref.name, Output: corechat.NewTextToolOutput(result), IsError: isError,
	}
}

// forgetToolEnds removes speculative external results whose canonical batch
// failed to commit. Their starts remain open so RunLost synthesis records
// incomplete calls rather than publishing results that persistence rejected.
func (r *reducer) forgetToolEnds(callIDs []string) {
	for _, callID := range callIDs {
		ref, _ := r.tools.get(callID)
		if ref == nil {
			continue
		}
		ref.end = nil
		ref.finishedAt = time.Time{}
	}
}

func (r *reducer) completeTool(ref *openTool, e ToolCallFinished) ([]ProjectionEvent, error) {
	var out []ProjectionEvent
	if e.OutputText != "" {
		delta, err := newToolOutputItemDelta(e.OutputText)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemChanged{
			ItemID: ref.id,
			Delta:  delta,
		})
	}
	arguments := ref.arguments
	if e.Arguments != "" {
		parsed, err := parseToolArguments(e.Arguments)
		if err != nil {
			return nil, fmt.Errorf("tool %q effective arguments: %w", ref.name, err)
		}
		arguments = parsed
	}
	invocation := newToolInvocation(ref.name, arguments, e.Result)
	invocation.Offload = e.Offload
	item, err := r.runningToolItem(ref)
	if err != nil {
		return nil, err
	}
	if e.Failure != nil {
		if validateErr := e.Failure.Validate(); validateErr != nil {
			return nil, fmt.Errorf("tool %q failure: %w", ref.name, validateErr)
		}
		item, err = item.FailToolCall(*invocation, *e.Failure, ref.attemptStartedAt, ref.finishedAt)
	} else {
		item, err = item.CompleteToolCall(*invocation, ref.attemptStartedAt, ref.finishedAt)
	}
	if err != nil {
		return nil, err
	}
	return append(out, ItemCompleted{Item: item, mutatedPaths: e.MutatedPaths}), nil
}

// usageProgress records the executor's latest accounting report and previews the
// Run's resulting total. The report is remembered rather than only forwarded:
// it is what the Run commits if the segment ends without a fresh one.
func (r *reducer) usageProgress(e UsageReported) ([]ProjectionEvent, error) {
	if err := r.applyUsage(SegmentUsage{
		Tokens:  e.TokenUsage,
		ByModel: e.ByModel,
		Cost:    e.Cost,
		Steps:   e.Steps,
	}); err != nil {
		return nil, err
	}
	metrics, err := r.metrics()
	if err != nil {
		return nil, err
	}
	usage, reported := metrics.Usage()
	progress := Progress{}
	if reported {
		progress.Usage = &usage
	}
	if e.ContextTokens > 0 {
		r.contextTokens = e.ContextTokens
		contextTokens := e.ContextTokens
		progress.ContextTokens = &contextTokens
	}
	step := r.step
	progress.Step = &step
	return []ProjectionEvent{SegmentProgressed{Progress: progress}}, nil
}

func (r *reducer) compaction(e CompactionBoundary) ([]ProjectionEvent, error) {
	dropped := max(e.MessagesBefore-e.MessagesAfter, 0)
	id, err := r.nextItemID()
	if err != nil {
		return nil, err
	}
	now := r.now()
	item, err := transcript.NewCompaction(r.itemIdentity(id, now), e.Summary, dropped)
	if err != nil {
		return nil, err
	}
	return []ProjectionEvent{ItemCompleted{Item: item}}, nil
}

func (r *reducer) openUserMessage() ([]ProjectionEvent, error) {
	if len(r.userInput) == 0 {
		return nil, nil
	}
	input := r.userInput
	r.userInput = nil
	if r.cfg.ModelOnlyInput {
		return nil, nil
	}
	id, now := userMessageItemID(r.cfg.SegmentID), r.now()
	item, err := transcript.NewUserMessage(r.itemIdentity(id, now), input)
	if err != nil {
		return nil, err
	}
	return []ProjectionEvent{ItemCompleted{Item: item}}, nil
}

func (r *reducer) steerMessagesApplied(e SteerMessagesApplied) ([]ProjectionEvent, error) {
	if len(e.Messages) == 0 {
		return nil, errors.New("applied steer batch is empty")
	}
	out, err := r.closeStreaming(transcript.MessageCommentary)
	if err != nil {
		return nil, err
	}
	for messageIndex, applied := range e.Messages {
		message := applied.Content
		if len(message) == 0 {
			return nil, fmt.Errorf("applied steer message %d is empty", messageIndex)
		}
		for blockIndex, block := range message {
			if err := block.Validate(); err != nil {
				return nil, fmt.Errorf(
					"applied steer message %d content %d: %w",
					messageIndex, blockIndex, err,
				)
			}
		}
		if applied.ProjectedItemID != "" {
			if _, err := resourceid.ParseItem(applied.ProjectedItemID); err != nil {
				return nil, fmt.Errorf("applied steer message %d projected Item: %w", messageIndex, err)
			}
			continue
		}
		id, identityErr := r.nextItemID()
		if identityErr != nil {
			return nil, identityErr
		}
		now := r.now()
		content := transcript.CloneContent(message)
		item, err := transcript.NewUserMessage(r.itemIdentity(id, now), content)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemCompleted{Item: item})
	}
	if len(out) == 0 {
		out = append(out, SegmentProgressed{Progress: Progress{Activity: applyingUserInputActivity}})
	}
	return out, nil
}

func (r *reducer) planSnapshot(e PlanUpdated) []ProjectionEvent {
	snapshot := r.planState(e.State)
	// Remembered so the segment can fence its final value: a client folding this
	// stream must reach segment.finished holding the Plan the segment ended with,
	// not the Plan as of whichever change happened to be published last.
	r.plan = &snapshot
	return []ProjectionEvent{snapshot}
}

func (r *reducer) planState(state plan.State) PlanSnapshot {
	return PlanSnapshot{
		SessionID: r.cfg.SessionID, Steps: state.Steps(),
		Revision: state.Revision(), UpdatedAt: state.UpdatedAt(),
	}
}
