package runs

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	corechat "github.com/Tangerg/scope/core/chat"
)

var (
	errExecutorContract = errors.New("runs: executor contract violation")
	errReducerInvariant = errors.New("runs: reducer invariant violation")
)

type reducerConfig struct {
	RunID             string
	SegmentID         string
	SessionID         string
	Lineage           run.Lineage
	CWD               string
	ExecutorID        string
	GoalIncarnationID string
	ModelSelection    modelref.Selection
	CreatedAt         time.Time
	UserInput         []transcript.ContentBlock
	// ConversationInput is the exact composed model message for a fresh root.
	// nil is reserved for continuation input, which has no composition layer.
	ConversationInput *corechat.Message
	// ModelOnlyInput suppresses only the opening userMessage Item. The same input
	// still enters the durable provider conversation in open(), so hiding Runtime
	// control material from the narrative cannot starve the model of instructions.
	ModelOnlyInput bool
	// Metrics is what the Run had already consumed before this segment opened —
	// zero for a first segment, the parked Run's accrual for a continuation. Every
	// Run record this reducer commits is the sum of this and the current segment,
	// so a resumed Run reports the Run rather than its latest continuation.
	Metrics run.Metrics
	// ContextTokens is the latest authoritative prompt footprint brought into a
	// resumed Segment. Zero means the Run has not observed one yet.
	ContextTokens int64
	// Limits is the allowance in force for the whole Run, frozen at admission and
	// carried unchanged through every continuation.
	Limits run.Limits
	// Capabilities is the Run's frozen optional behavior. Every record this reducer
	// commits carries the admission value, including continuation records.
	Capabilities run.Capabilities
	Continuation *treeContinuation
	Now          func() time.Time
	CancelReason func() string
}

// reducer is the per-segment state machine that turns executor events into the
// canonical Event family and EventCommit facts. It owns open item state,
// item identity, resume correlation, terminal synthesis, and error semantics.
type reducer struct {
	cfg     reducerConfig
	resume  *resumeBinding
	itemIDs segmentItemIdentities
	// step is the latest cumulative accounted model-call count reported by the
	// executor. It uses the same unit as Limits.MaxSteps; tool events never
	// infer it.
	step int
	// usage is the latest authoritative cumulative Run accounting reported by
	// the executor. Nil means this segment has not advanced the committed
	// snapshot in cfg.Metrics.
	usage           *accounting.Usage
	contextTokens   int64
	segmentDuration time.Duration
	userInput       []transcript.ContentBlock
	text            *openText
	reasoning       *openText
	modelCalls      map[string]time.Time
	// modelBoundaryClosed fences lossy stream observations that arrive after the
	// authoritative ModelCallCompleted commit. A later ModelCallStarted reopens
	// the observation window for the next provider turn.
	modelBoundaryClosed bool
	// Exactly one side of the final model/process confirmation handshake may be
	// pending. The two facts travel through different executor paths and either
	// can arrive first; only ModelCallCompleted projects transcript content.
	lastModelMessage      *corechat.Message
	earlyAssistantMessage *corechat.Message
	toolCallIDs           map[string]struct{}
	toolPositions         map[toolPosition]string
	tools                 openTools
	drained               []DrainedTool
	errFailure            *run.Failure
	// plan is the last Plan this segment published, kept so the segment
	// can fence its final value before finishing. Nil means this segment never
	// changed the projection, and a segment that changed nothing has nothing to
	// fence.
	plan *PlanSnapshot
	// toolContext mirrors only this root Segment's provider-neutral assistant
	// ToolCall and ToolResult messages. It lets a terminal boundary close calls
	// the model committed even when cancellation won before ToolCallStarted.
	// The durable Conversation remains owned by the conversation store; this
	// immutable aggregate is only the reducer's atomic projection ledger.
	toolContext conversation.Conversation
}

type openTool struct {
	callID            string
	sourceCallID      string
	modelCallSequence uint32
	toolCallIndex     uint32
	id                string
	occurredAt        time.Time
	attemptStartedAt  time.Time
	finishedAt        time.Time
	name              string
	arguments         tool.Arguments
	safetyClass       tool.SafetyClass
	approvalDecision  approval.Decision
	end               *ToolCallFinished
}

type toolPosition struct {
	modelCallSequence uint32
	toolCallIndex     uint32
}

func newReducer(cfg reducerConfig) *reducer {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	cfg.Now = now
	// The reducer outlives the Start request and publishes UserInput through the
	// journal after admission. Own the slice before it becomes persisted/live
	// state so a caller reusing its command buffer cannot rewrite emitted facts.
	cfg.UserInput = slices.Clone(cfg.UserInput)
	if cfg.ConversationInput != nil {
		message := cfg.ConversationInput.Clone()
		cfg.ConversationInput = &message
	}
	var resume *resumeBinding
	if cfg.Continuation != nil {
		resume = resumeBindingFrom(*cfg.Continuation, cfg.RunID)
	}
	return &reducer{
		cfg: cfg, resume: resume, itemIDs: newSegmentItemIdentities(cfg.SegmentID),
		userInput: transcript.CloneContent(cfg.UserInput),
		step:      cfg.Metrics.Steps(), contextTokens: cfg.ContextTokens,
		modelCalls: make(map[string]time.Time), toolCallIDs: make(map[string]struct{}),
		toolPositions: make(map[toolPosition]string), tools: newOpenTools(),
	}
}

// clone creates the speculative reducer used by an authoritative fact commit.
// The Run pump swaps it in only after the complete persistence batch succeeds;
// a rejected write therefore cannot consume model/tool state or mint identities
// that the durable projection never observed.
func (r *reducer) clone() *reducer {
	if r == nil {
		return nil
	}
	cloned := *r
	cloned.cfg.UserInput = slices.Clone(r.cfg.UserInput)
	if r.cfg.ConversationInput != nil {
		message := r.cfg.ConversationInput.Clone()
		cloned.cfg.ConversationInput = &message
	}
	cloned.userInput = transcript.CloneContent(r.userInput)
	cloned.modelCalls = maps.Clone(r.modelCalls)
	cloned.toolCallIDs = maps.Clone(r.toolCallIDs)
	cloned.toolPositions = maps.Clone(r.toolPositions)
	cloned.drained = slices.Clone(r.drained)
	cloned.tools = r.tools.clone()
	cloned.text = r.text.clone()
	cloned.reasoning = r.reasoning.clone()
	cloned.resume = cloneResumeBinding(r.resume)
	if r.plan != nil {
		plan := *r.plan
		plan.Steps = slices.Clone(r.plan.Steps)
		cloned.plan = &plan
	}
	if r.errFailure != nil {
		failure := *r.errFailure
		cloned.errFailure = &failure
	}
	if r.lastModelMessage != nil {
		message := r.lastModelMessage.Clone()
		cloned.lastModelMessage = &message
	}
	if r.earlyAssistantMessage != nil {
		message := r.earlyAssistantMessage.Clone()
		cloned.earlyAssistantMessage = &message
	}
	return &cloned
}

func cloneResumeBinding(value *resumeBinding) *resumeBinding {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.callItems = maps.Clone(value.callItems)
	cloned.toolItems = maps.Clone(value.toolItems)
	cloned.byName = maps.Clone(value.byName)
	cloned.drained = slices.Clone(value.drained)
	cloned.committed = maps.Clone(value.committed)
	cloned.consumed = maps.Clone(value.consumed)
	return &cloned
}

func (r *reducer) nextItemID() (string, error) { return r.itemIDs.Next() }

func (r *reducer) open() (reductionBatch, error) {
	if r.resume != nil && r.resume.err != nil {
		return reductionBatch{}, fmt.Errorf("%w: %w", errReducerInvariant, r.resume.err)
	}
	// The opening Run record goes through runRecord like every other one, so a
	// resumed segment announces the Run's accrual and allowance rather than a fresh
	// Run's zeros. Only the creation stamp differs: an opening may have to mint one.
	opening, err := r.runRecord(run.Running)
	if err != nil {
		return reductionBatch{}, err
	}
	out := []ProjectionEvent{SegmentStarted{Run: opening}}
	userMessage, err := r.openUserMessage()
	if err != nil {
		return reductionBatch{}, err
	}
	out = append(out, userMessage...)
	batch, err := r.project(out)
	if err != nil {
		return reductionBatch{}, err
	}
	if r.cfg.Lineage.IsRoot() && r.cfg.ConversationInput != nil {
		message := r.cfg.ConversationInput.Clone()
		if message.Role != corechat.RoleUser || message.Validate() != nil || conversation.ValidateMessageIdentities(message) != nil {
			return reductionBatch{}, fmt.Errorf("%w: opening conversation input is not a valid User message", errReducerInvariant)
		}
		if err := r.attachConversationMessages(&batch, []corechat.Message{message}); err != nil {
			return reductionBatch{}, err
		}
	}
	return batch, nil
}

func (r *reducer) reduce(ev ExecutionFact) (reductionBatch, error) {
	reduced, err := r.reduceFact(ev)
	if err != nil {
		return reductionBatch{}, err
	}
	return r.projectFact(reduced)
}

func (r *reducer) reduceFact(ev ExecutionFact) (factReduction, error) {
	switch e := ev.(type) {
	case MessageDelta:
		// Reasoning and message are two concurrent projections of one model
		// response, not mutually exclusive stream modes. Keep each Item open until
		// ModelCallCompleted supplies the authoritative full message; completing one
		// merely because the other emitted would make that final message duplicate it.
		if r.modelBoundaryClosed {
			return factReduction{}, nil
		}
		appended, err := r.appendText(e.Text)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: append text: %w", errReducerInvariant, err)
		}
		return factReduction{events: appended}, nil
	case ReasoningDelta:
		if r.modelBoundaryClosed {
			return factReduction{}, nil
		}
		appended, err := r.appendReasoning(e.Text)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: append reasoning: %w", errReducerInvariant, err)
		}
		return factReduction{events: appended}, nil
	case AssistantMessageCompleted:
		return r.reduceAssistantMessage(e)
	case ModelCallStarted:
		return r.startModelCall(e)
	case ModelCallCompleted:
		return r.completeModelCall(e)
	case ModelCallFailed:
		return r.failModelCall(e)
	case ToolCallStarted:
		return r.startToolCall(e)
	case ToolCallFinished:
		return r.finishToolCall(e)
	case UsageReported:
		events, err := r.usageProgress(e)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: usage report: %w", errExecutorContract, err)
		}
		return factReduction{events: events}, nil
	case SteerMessagesApplied:
		events, err := r.steerMessagesApplied(e)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: applied steers: %w", errExecutorContract, err)
		}
		var messages []corechat.Message
		if r.cfg.Lineage.IsRoot() {
			messages = make([]corechat.Message, len(e.Messages))
			for index, applied := range e.Messages {
				message, err := MaterializeUserMessage(applied.Content)
				if err != nil {
					return factReduction{}, fmt.Errorf("%w: applied steer message[%d]: %w", errExecutorContract, index, err)
				}
				messages[index] = message
			}
		}
		return factReduction{events: events, conversationMessages: messages}, nil
	case PlanUpdated:
		return factReduction{events: r.planSnapshot(e)}, nil
	case CompactionBoundary:
		events, err := r.compaction(e)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: compaction: %w", errReducerInvariant, err)
		}
		return factReduction{events: events}, nil
	case SegmentInterrupted:
		interrupted, err := r.interrupt(e)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: interrupt: %w", errExecutorContract, err)
		}
		return interrupted, nil
	case SegmentEnded:
		return r.endSegment(e)
	default:
		return factReduction{}, fmt.Errorf("%w: unhandled event %T", errExecutorContract, ev)
	}
}

func (r *reducer) reduceAssistantMessage(completed AssistantMessageCompleted) (factReduction, error) {
	if r.lastModelMessage != nil {
		if !reflect.DeepEqual(*r.lastModelMessage, completed.Message) {
			return factReduction{}, fmt.Errorf(
				"%w: executor final assistant message differs from the last committed model response",
				errExecutorContract,
			)
		}
		r.lastModelMessage = nil
		return factReduction{}, nil
	}
	if r.earlyAssistantMessage != nil {
		return factReduction{}, fmt.Errorf(
			"%w: executor completed the assistant message more than once",
			errExecutorContract,
		)
	}
	message := completed.Message.Clone()
	r.earlyAssistantMessage = &message
	return factReduction{}, nil
}

func (r *reducer) startModelCall(started ModelCallStarted) (factReduction, error) {
	if _, err := runtimeidentity.ParseEffect(started.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call start: %v", errExecutorContract, err)
	}
	if _, duplicate := r.modelCalls[started.CallID]; duplicate {
		return factReduction{}, fmt.Errorf("%w: model call %q started more than once", errExecutorContract, started.CallID)
	}
	startedAt := r.now()
	r.modelCalls[started.CallID] = startedAt
	r.modelBoundaryClosed = false
	return factReduction{
		events: []ProjectionEvent{SegmentProgressed{Progress: Progress{Activity: "Calling model"}}},
		modelInvocations: []ModelInvocationCommit{{
			CallID: started.CallID, SegmentID: r.cfg.SegmentID,
			State: ModelInvocationStarted, StartedAt: startedAt,
		}},
	}, nil
}

func (r *reducer) completeModelCall(completed ModelCallCompleted) (factReduction, error) {
	if _, err := runtimeidentity.ParseEffect(completed.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call completion: %v", errExecutorContract, err)
	}
	startedAt, started := r.modelCalls[completed.CallID]
	if !started {
		return factReduction{}, fmt.Errorf("%w: model call %q completed without a start", errExecutorContract, completed.CallID)
	}
	if err := conversation.ValidateMessageIdentities(completed.Message); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call completion: %v", errExecutorContract, err)
	}
	finishedAt := r.now()
	if finishedAt.Before(startedAt) {
		return factReduction{}, fmt.Errorf(
			"%w: model call %q completion precedes its start",
			errExecutorContract,
			completed.CallID,
		)
	}
	delete(r.modelCalls, completed.CallID)
	r.modelBoundaryClosed = true
	if r.earlyAssistantMessage != nil && !reflect.DeepEqual(*r.earlyAssistantMessage, completed.Message) {
		return factReduction{}, fmt.Errorf(
			"%w: executor final assistant message differs from the committed model response",
			errExecutorContract,
		)
	}
	phase := transcript.MessageFinalAnswer
	if messageRequestsToolCalls(completed.Message) {
		phase = transcript.MessageCommentary
	}
	events, err := r.completeModelMessage(completed.Message, phase)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call completion: %w", errExecutorContract, err)
	}
	if r.earlyAssistantMessage != nil {
		r.earlyAssistantMessage = nil
		r.lastModelMessage = nil
	} else if !messageRequestsToolCalls(completed.Message) {
		message := completed.Message.Clone()
		r.lastModelMessage = &message
	} else {
		r.lastModelMessage = nil
	}
	progressEvents, err := r.usageProgress(UsageReported{
		TokenUsage: completed.TokenUsage, ByModel: completed.ByModel, Cost: completed.Cost,
		Steps: completed.Steps, ContextTokens: completed.ContextTokens,
	})
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call usage: %w", errExecutorContract, err)
	}
	metrics, err := r.metrics()
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call metrics: %w", errExecutorContract, err)
	}
	conversationMessages := r.rootConversationMessages(completed.Message)
	if err := r.appendToolContext(conversationMessages); err != nil {
		return factReduction{}, fmt.Errorf("%w: track model Tool context: %w", errReducerInvariant, err)
	}
	return factReduction{
		events:               append(events, progressEvents...),
		conversationMessages: conversationMessages,
		modelInvocations: []ModelInvocationCommit{{
			CallID: completed.CallID, SegmentID: r.cfg.SegmentID,
			State: ModelInvocationCompleted, StartedAt: startedAt, FinishedAt: finishedAt,
		}},
		progress: &ProgressCommit{
			SegmentID: r.cfg.SegmentID, Metrics: metrics,
			ContextTokens: r.contextTokens, UpdatedAt: finishedAt,
		},
	}, nil
}

func (r *reducer) failModelCall(failed ModelCallFailed) (factReduction, error) {
	if _, err := runtimeidentity.ParseEffect(failed.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call failure: %v", errExecutorContract, err)
	}
	startedAt, started := r.modelCalls[failed.CallID]
	if !started {
		return factReduction{}, fmt.Errorf("%w: model call %q failed without a start", errExecutorContract, failed.CallID)
	}
	finishedAt := r.now()
	if finishedAt.Before(startedAt) {
		return factReduction{}, fmt.Errorf(
			"%w: model call %q failure precedes its start",
			errExecutorContract,
			failed.CallID,
		)
	}
	delete(r.modelCalls, failed.CallID)
	return factReduction{
		events: []ProjectionEvent{SegmentProgressed{Progress: Progress{Activity: "Model call failed"}}},
		modelInvocations: []ModelInvocationCommit{{
			CallID: failed.CallID, SegmentID: r.cfg.SegmentID,
			State: ModelInvocationFailed, StartedAt: startedAt, FinishedAt: finishedAt,
		}},
	}, nil
}

func (r *reducer) startToolCall(started ToolCallStarted) (factReduction, error) {
	events, err := r.toolStart(started)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: tool call start: %w", errExecutorContract, err)
	}
	ref, _ := r.tools.get(started.CallID)
	if ref == nil {
		return factReduction{}, fmt.Errorf("%w: started Tool %q has no open projection", errReducerInvariant, started.CallID)
	}
	running, err := r.runningToolItem(ref)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: started Tool %q projection: %w", errReducerInvariant, started.CallID, err)
	}
	reduced := factReduction{events: events, items: []transcript.Item{running}}
	if ref.modelCallSequence > 0 {
		if err := r.trackStartedToolCall(started); err != nil {
			return factReduction{}, fmt.Errorf("%w: track started Tool context: %w", errReducerInvariant, err)
		}
		reduced.toolInvocations = []ToolInvocationCommit{{
			CallID: ref.callID, ItemID: ref.id, SegmentID: r.cfg.SegmentID,
			State: ToolInvocationStarted, StartedAt: ref.attemptStartedAt,
		}}
	}
	return reduced, nil
}

func (r *reducer) finishToolCall(finished ToolCallFinished) (factReduction, error) {
	events, invocations, messages, err := r.toolEnd(finished)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: tool call end: %w", errExecutorContract, err)
	}
	settledCallIDs := make([]string, len(invocations))
	for index, invocation := range invocations {
		settledCallIDs[index] = invocation.CallID
	}
	if err := r.appendToolContext(messages); err != nil {
		return factReduction{}, fmt.Errorf("%w: track completed Tool context: %w", errReducerInvariant, err)
	}
	return factReduction{
		events: events, conversationMessages: messages,
		toolInvocations: invocations, settledToolCallIDs: settledCallIDs,
	}, nil
}

func (r *reducer) endSegment(ended SegmentEnded) (factReduction, error) {
	if len(r.modelCalls) > 0 && ended.Reason != run.OutcomeLost {
		return factReduction{}, fmt.Errorf(
			"%w: segment ended with %d unsettled model calls",
			errExecutorContract,
			len(r.modelCalls),
		)
	}
	modelInvocations, err := r.closeLostModelCalls(ended.Reason)
	if err != nil {
		return factReduction{}, err
	}
	if trackUnconsumedResumeToolCallsErr := r.trackUnconsumedResumeToolCalls(); trackUnconsumedResumeToolCallsErr != nil {
		return factReduction{}, fmt.Errorf("%w: track resumed Tool context: %w", errReducerInvariant, trackUnconsumedResumeToolCallsErr)
	}
	openTools := r.tools.ordered()
	events, err := r.segmentEnd(ended)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: segment end: %w", errExecutorContract, err)
	}
	closure, err := r.closeOpenToolContext(
		terminalToolResult(ended.Reason, r.cancelReason()),
		completedTerminalToolResults(openTools),
	)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: close terminal Tool context: %w", errReducerInvariant, err)
	}
	return factReduction{
		events:               events,
		conversationMessages: closure,
		modelInvocations:     modelInvocations,
		toolInvocations:      closedToolInvocationCommits(r.cfg.SegmentID, openTools),
	}, nil
}

func (r *reducer) closeLostModelCalls(outcome run.Outcome) ([]ModelInvocationCommit, error) {
	if outcome != run.OutcomeLost {
		return nil, nil
	}
	finishedAt := r.now()
	callIDs := slices.Sorted(maps.Keys(r.modelCalls))
	invocations := make([]ModelInvocationCommit, 0, len(callIDs))
	for _, callID := range callIDs {
		startedAt := r.modelCalls[callID]
		if finishedAt.Before(startedAt) {
			return nil, fmt.Errorf("%w: model call %q loss precedes its start", errExecutorContract, callID)
		}
		invocations = append(invocations, ModelInvocationCommit{
			CallID: callID, SegmentID: r.cfg.SegmentID,
			State: ModelInvocationUnknown, StartedAt: startedAt, FinishedAt: finishedAt,
		})
	}
	clear(r.modelCalls)
	return invocations, nil
}

func (r *reducer) rootConversationMessages(messages ...corechat.Message) []corechat.Message {
	if !r.cfg.Lineage.IsRoot() {
		return nil
	}
	return appendClonedMessages(nil, messages...)
}

func appendClonedMessages(dst []corechat.Message, messages ...corechat.Message) []corechat.Message {
	for _, message := range messages {
		dst = append(dst, message.Clone())
	}
	return dst
}

func closedToolInvocationCommits(segmentID string, tools []*openTool) []ToolInvocationCommit {
	commits := make([]ToolInvocationCommit, 0, len(tools))
	for _, ref := range tools {
		if ref == nil || ref.modelCallSequence == 0 || ref.finishedAt.IsZero() {
			continue
		}
		state := ToolInvocationIncomplete
		if ref.end != nil {
			state = ToolInvocationCompleted
		}
		commits = append(commits, ToolInvocationCommit{
			CallID: ref.callID, ItemID: ref.id, SegmentID: segmentID,
			State: state, StartedAt: ref.attemptStartedAt, FinishedAt: ref.finishedAt,
		})
	}
	return commits
}

func (r *reducer) synthesizeTerminal() (reductionBatch, error) {
	if err := r.trackUnconsumedResumeToolCalls(); err != nil {
		return reductionBatch{}, fmt.Errorf("%w: track resumed Tool context: %w", errReducerInvariant, err)
	}
	out, err := r.closeStreaming(transcript.MessageCommentary)
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: close streaming: %w", errReducerInvariant, err)
	}
	resumedTools, err := r.abandonUnconsumedResumeTools()
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: abandon resumed tools: %w", errReducerInvariant, err)
	}
	out = append(out, resumedTools...)
	openTools := r.tools.ordered()
	drained, err := r.drainTools()
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: drain tools: %w", errReducerInvariant, err)
	}
	out = append(out, drained...)
	// No SegmentEnded arrived, so nothing fresh was reported: the Segment's accrual
	// stands as last reported and is committed as-is.
	var failure *run.Failure
	var modelInvocations []ModelInvocationCommit
	outcome := run.OutcomeCanceled
	if len(r.modelCalls) > 0 {
		outcome = run.OutcomeLost
		failure = &run.Failure{
			Kind:   run.FailureLost,
			Detail: "a model invocation ended without a provable durable result",
		}
		finishedAt := r.now()
		callIDs := slices.Sorted(maps.Keys(r.modelCalls))
		modelInvocations = make([]ModelInvocationCommit, 0, len(callIDs))
		for _, callID := range callIDs {
			startedAt := r.modelCalls[callID]
			if finishedAt.Before(startedAt) {
				return reductionBatch{}, fmt.Errorf("%w: model call %q loss precedes its start", errReducerInvariant, callID)
			}
			modelInvocations = append(modelInvocations, ModelInvocationCommit{
				CallID: callID, SegmentID: r.cfg.SegmentID,
				State: ModelInvocationUnknown, StartedAt: startedAt, FinishedAt: finishedAt,
			})
		}
		clear(r.modelCalls)
	} else if r.errFailure != nil {
		outcome = run.OutcomeFailed
		failure = r.errFailure
	}
	detail := ""
	if outcome == run.OutcomeCanceled && r.cfg.CancelReason != nil {
		detail = r.cfg.CancelReason()
	}
	terminal, err := r.finishedRun(outcome, failure, detail)
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: synthesize terminal: %w", errReducerInvariant, err)
	}
	out = append(out, terminal)
	closure, err := r.closeOpenToolContext(
		terminalToolResult(outcome, detail),
		completedTerminalToolResults(openTools),
	)
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: close synthesized Tool context: %w", errReducerInvariant, err)
	}
	batch, err := r.project(out)
	if err != nil {
		return reductionBatch{}, err
	}
	if err := r.attachDurableObservation(
		&batch,
		closure,
		modelInvocations,
		closedToolInvocationCommits(r.cfg.SegmentID, openTools),
		nil,
	); err != nil {
		return reductionBatch{}, err
	}
	return batch, nil
}

// abandonUnconsumedResumeTools closes logical Tool Items that were carried
// across a human boundary but whose executor attempt never restarted in this
// Segment. Without this step an activation failure or a cancel-before-activate
// can terminalize the Run while leaving its preexisting Tool Item running.
func (r *reducer) abandonUnconsumedResumeTools() ([]ProjectionEvent, error) {
	if r.resume == nil {
		return nil, nil
	}
	remaining := r.resume.remainingDrainedTools()
	events := make([]ProjectionEvent, 0, len(remaining))
	for _, drained := range remaining {
		arguments, err := parseToolArguments(drained.Arguments)
		if err != nil {
			return nil, fmt.Errorf("tool %q arguments: %w", drained.Name, err)
		}
		ref := &openTool{
			callID:           drained.CallID,
			sourceCallID:     drained.SourceCallID,
			id:               drained.ItemID,
			occurredAt:       drained.ItemOccurredAt,
			name:             drained.Name,
			arguments:        arguments,
			approvalDecision: r.resume.approvalDecision(drained.ItemID),
		}
		completed, err := r.abandonUnstartedToolItem(ref)
		if err != nil {
			return nil, err
		}
		events = append(events, completed)
		r.resume.consumeToolItem(drained.ItemID)
	}
	return events, nil
}

// abort marks the Segment as failed so terminal synthesis produces an error
// outcome. It takes no cause: an internal failure exposes only its stable problem
// kind to observers.
// That makes the caller's span the only place the cause survives — a rejected
// terminal commit or a contract-violating executor event is otherwise invisible
// — so every caller records it there before calling this.
func (r *reducer) abort() {
	r.errFailure = &run.Failure{Kind: run.FailureInternal}
}

func (r *reducer) now() time.Time {
	now := r.cfg.Now().UTC()
	createdAt := r.cfg.CreatedAt.UTC()
	if !createdAt.IsZero() && now.Before(createdAt) {
		return createdAt
	}
	return now
}
