package runs

import (
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/optional"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	corechat "github.com/Tangerg/scope/core/chat"
)

var (
	errExecutorContract = errors.New("runs: executor contract violation")
	errReducerInvariant = errors.New("runs: reducer invariant violation")
)

type reducerConfig struct {
	// Opened is the Run exactly as this Segment's opening admits or resumes it.
	// Every record the reducer commits is a domain transition from it, so the
	// Run's identity, provenance, and prior accrual have this one source.
	Opened       run.Run
	WorkspaceCWD string
	Isolated     bool
	UserInput    []transcript.ContentBlock
	// ConversationInput is the exact composed model message for a fresh root.
	// nil is reserved for continuation input, which has no composition layer.
	ConversationInput *corechat.Message
	// ModelOnlyInput suppresses only the opening userMessage Item. The same input
	// still enters the durable provider conversation in open(), so hiding Runtime
	// control material from the narrative cannot starve the model of instructions.
	ModelOnlyInput bool
	Continuation   *treeContinuation
	Now            func() time.Time
	CancelReason   func() string
}

// reducer is the per-segment state machine that turns executor events into the
// canonical Event family and EventCommit facts. It owns open item state,
// item identity, resume correlation, terminal synthesis, and error semantics.
type reducer struct {
	cfg     reducerConfig
	resume  *resumeBinding
	itemIDs segmentItemIdentities
	// step is the latest cumulative accounted model-call count reported by the
	// executor. Tool events never
	// infer it.
	step int
	// usage is the latest authoritative cumulative Run accounting reported by
	// the executor. Nil means this segment has not advanced the committed
	// accrual the opened Run brought in.
	usage           *accounting.Usage
	contextTokens   int64
	segmentDuration time.Duration
	userInput       []transcript.ContentBlock
	text            *openText
	reasoning       *openText
	modelCalls      map[string]time.Time
	// modelBoundaryClosed fences lossy stream observations that arrive after the
	// authoritative model completion or failure commit. A later ModelCallStarted reopens
	// the observation window for the next provider turn.
	modelBoundaryClosed bool
	toolCallIDs         map[string]struct{}
	toolPositions       map[toolPosition]string
	tools               openTools
	drained             []DrainedTool
	errFailure          *run.Failure
	// plan is the last Plan this segment published, kept so the segment
	// can fence its final value before finishing. Nil means this segment never
	// changed the projection, and a segment that changed nothing has nothing to
	// fence.
	plan *PlanSnapshot
}

type openTool struct {
	callID            string
	sourceCallID      string
	modelCallSequence uint64
	toolCallIndex     uint32
	item              transcript.Item
	// The executor's admitted attempt input can differ from the reviewed Item
	// after edits or hooks. Settlement commits this observation into the Item.
	attemptInvocation transcript.ToolInvocation
	attemptStartedAt  time.Time
	finishedAt        time.Time
}

type toolPosition struct {
	modelCallSequence uint64
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
		resume = resumeBindingFrom(*cfg.Continuation, cfg.Opened.ID())
	}
	return &reducer{
		cfg: cfg, resume: resume, itemIDs: newSegmentItemIdentities(cfg.Opened.ActiveSegmentID()),
		userInput: transcript.CloneContent(cfg.UserInput),
		step:      cfg.Opened.Metrics().Steps(), contextTokens: cfg.Opened.ContextTokens(),
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
	cloned.errFailure = optional.Clone(r.errFailure)
	return &cloned
}

func cloneResumeBinding(value *resumeBinding) *resumeBinding {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.callItems = maps.Clone(value.callItems)
	cloned.drained = slices.Clone(value.drained)
	return &cloned
}

func (r *reducer) nextItemID() (string, error) { return r.itemIDs.Next() }

func (r *reducer) open() (reductionBatch, error) {
	if r.resume != nil && r.resume.err != nil {
		return reductionBatch{}, fmt.Errorf("%w: %w", errReducerInvariant, r.resume.err)
	}
	out := []ProjectionEvent{SegmentStarted{Run: r.cfg.Opened}}
	if r.cfg.Continuation != nil {
		for _, answered := range r.cfg.Continuation.answeredQuestionsFor(r.cfg.Opened.ID()) {
			out = append(out, QuestionAnswered{Item: answered})
		}
	}
	userMessage, err := r.openUserMessage()
	if err != nil {
		return reductionBatch{}, err
	}
	out = append(out, userMessage...)
	batch, err := r.project(out)
	if err != nil {
		return reductionBatch{}, err
	}
	if r.cfg.Opened.Lineage().IsRoot() && r.cfg.ConversationInput != nil {
		message := r.cfg.ConversationInput.Clone()
		if message.Role != corechat.RoleUser || message.Validate() != nil {
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
	case ModelCallStarted:
		return r.startModelCall(e)
	case ModelCallCompleted:
		return r.completeModelCall(e)
	case ModelCallFailed:
		return r.failModelCall(e)
	case ToolCallStarted:
		return r.startToolCall(e)
	case ToolResultsCommitted:
		return r.finishToolResults(e)
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
		if r.cfg.Opened.Lineage().IsRoot() {
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

func (r *reducer) startModelCall(started ModelCallStarted) (factReduction, error) {
	if err := runtimeidentity.ValidateEffect(started.CallID); err != nil {
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
			CallID: started.CallID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: ModelInvocationStarted, StartedAt: startedAt,
		}},
	}, nil
}

func (r *reducer) completeModelCall(completed ModelCallCompleted) (factReduction, error) {
	if err := runtimeidentity.ValidateEffect(completed.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call completion: %v", errExecutorContract, err)
	}
	startedAt, started := r.modelCalls[completed.CallID]
	if !started {
		return factReduction{}, fmt.Errorf("%w: model call %q completed without a start", errExecutorContract, completed.CallID)
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
	var events []ProjectionEvent
	var conversationMessages []corechat.Message
	if completed.Message != nil {
		message := *completed.Message
		phase := transcript.MessageFinalAnswer
		if messageRequestsToolCalls(message) {
			phase = transcript.MessageCommentary
		}
		var err error
		events, err = r.completeModelMessage(message, phase)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: model call completion: %w", errExecutorContract, err)
		}
		conversationMessages = r.rootConversationMessages(message)
	}
	progressEvents, err := r.usageProgress(UsageReported{
		Tokens: completed.Tokens, ByModel: completed.ByModel, Cost: completed.Cost,
		Steps: completed.Steps, ContextTokens: completed.ContextTokens,
	})
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call usage: %w", errExecutorContract, err)
	}
	progressed, err := r.progressedRun(finishedAt)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: model call progress: %w", errExecutorContract, err)
	}
	return factReduction{
		events:               append(events, progressEvents...),
		conversationMessages: conversationMessages,
		modelInvocations: []ModelInvocationCommit{{
			CallID: completed.CallID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: ModelInvocationCompleted, StartedAt: startedAt, FinishedAt: finishedAt, Usage: completed.ReportedUsage,
			FirstOutputLatencyMillis: completed.FirstOutputLatencyMillis,
		}},
		progress: &progressed,
	}, nil
}

func (r *reducer) failModelCall(failed ModelCallFailed) (factReduction, error) {
	if err := runtimeidentity.ValidateEffect(failed.CallID); err != nil {
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
	events, err := r.failModelObservation(failed.Observation)
	if err != nil {
		return factReduction{}, err
	}
	delete(r.modelCalls, failed.CallID)
	r.modelBoundaryClosed = true
	return factReduction{
		events: append(events, SegmentProgressed{Progress: Progress{Activity: "Model call failed"}}),
		modelInvocations: []ModelInvocationCommit{{
			CallID: failed.CallID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: ModelInvocationFailed, StartedAt: startedAt, FinishedAt: finishedAt,
			FirstOutputLatencyMillis: failed.FirstOutputLatencyMillis,
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
	reduced := factReduction{events: events, items: []transcript.Item{ref.item}}
	if ref.modelCallSequence > 0 {
		reduced.toolInvocations = []ToolInvocationCommit{{
			CallID: ref.callID, ItemID: ref.item.ID(), SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: ToolInvocationStarted, StartedAt: ref.attemptStartedAt,
		}}
	}
	return reduced, nil
}

func (r *reducer) finishToolCall(finished ToolCallFinished) (factReduction, error) {
	events, invocations, err := r.toolEnd(finished)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: tool call end: %w", errExecutorContract, err)
	}
	return factReduction{
		events:          events,
		toolInvocations: invocations,
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
	openTools := r.tools.ordered()
	events, err := r.segmentEnd(ended)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: segment end: %w", errExecutorContract, err)
	}
	return factReduction{
		events:           events,
		modelInvocations: modelInvocations,
		toolInvocations:  closedToolInvocationCommits(r.cfg.Opened.ActiveSegmentID(), openTools),
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
			CallID: callID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: ModelInvocationUnknown, StartedAt: startedAt, FinishedAt: finishedAt,
		})
	}
	clear(r.modelCalls)
	return invocations, nil
}

func (r *reducer) rootConversationMessages(messages ...corechat.Message) []corechat.Message {
	if !r.cfg.Opened.Lineage().IsRoot() {
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
		commits = append(commits, ToolInvocationCommit{
			CallID: ref.callID, ItemID: ref.item.ID(), SegmentID: segmentID,
			State: state, StartedAt: ref.attemptStartedAt, FinishedAt: ref.finishedAt,
		})
	}
	return commits
}

func (r *reducer) synthesizeTerminal() (reductionBatch, error) {
	out, err := r.closeStreaming(transcript.MessageCommentary)
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: close streaming: %w", errReducerInvariant, err)
	}
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
				CallID: callID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
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
	terminal, err := r.finishedRun(outcome, failure, detail, nil)
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: synthesize terminal: %w", errReducerInvariant, err)
	}
	out = append(out, terminal)
	batch, err := r.project(out)
	if err != nil {
		return reductionBatch{}, err
	}
	if err := r.attachDurableObservation(
		&batch,
		nil,
		modelInvocations,
		closedToolInvocationCommits(r.cfg.Opened.ActiveSegmentID(), openTools),
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
		resumed := r.resume.callItems[drained.CallID]
		ref := &openTool{
			callID:       drained.CallID,
			sourceCallID: drained.SourceCallID,
			item:         resumed,
		}
		completed, err := r.abandonUnstartedToolItem(ref)
		if err != nil {
			return nil, err
		}
		events = append(events, completed)
		r.resume.consumeToolCall(drained.CallID)
	}
	return events, nil
}

// abort retains the first failure through terminal cleanup and persistence retries.
func (r *reducer) abort(cause error) {
	if r.errFailure == nil {
		r.errFailure = &run.Failure{Kind: run.FailureInternal, Detail: cause.Error()}
	}
}

func (r *reducer) now() time.Time {
	now := r.cfg.Now().UTC()
	if opened := r.cfg.Opened.UpdatedAt(); now.Before(opened) {
		return opened
	}
	return now
}

func (r *reducer) finishToolResults(batch ToolResultsCommitted) (factReduction, error) {
	if err := batch.Publication.Validate(); err != nil {
		return factReduction{}, err
	}
	if len(batch.Results) == 0 || len(batch.Starts) != len(batch.Results) {
		return factReduction{}, errors.New("runs: result publication requires its complete call set")
	}
	sequence := batch.Starts[0].ModelCallSequence
	if sequence == 0 {
		return factReduction{}, errors.New("runs: result publication requires model attribution")
	}
	var reduced factReduction
	for index, start := range batch.Starts {
		if start.ModelCallSequence != sequence || (index > 0 && start.ToolCallIndex <= batch.Starts[index-1].ToolCallIndex) || start.CallID != batch.Results[index].CallID {
			return factReduction{}, errors.New("runs: result publication call set is not in declared order")
		}
		if ref, open := r.tools.get(start.CallID); open {
			invocation, _ := ref.item.ToolInvocation()
			if ref.modelCallSequence != sequence || ref.toolCallIndex != start.ToolCallIndex || ref.sourceCallID != start.SourceCallID || invocation.Name != start.ToolName {
				return factReduction{}, errors.New("runs: result publication differs from its open call")
			}
			continue
		}
		started, err := r.startToolCall(start)
		if err != nil {
			return factReduction{}, err
		}
		// The final Item and journal row supersede the running projection within
		// this same transaction.
		reduced.events = append(reduced.events, started.events...)
	}
	for _, result := range batch.Results {
		finished, err := r.finishToolCall(result)
		if err != nil {
			return factReduction{}, err
		}
		reduced.toolResults = append(reduced.toolResults, result.ModelResult.Clone())
		reduced.events = append(reduced.events, finished.events...)
		reduced.toolInvocations = append(reduced.toolInvocations, finished.toolInvocations...)
	}
	if len(reduced.toolInvocations) != len(batch.Results) {
		return factReduction{}, errors.New("runs: result publication did not settle its entire call set")
	}
	if len(batch.ModelResults) > 0 {
		reduced.conversationMessages = r.rootConversationMessages(corechat.NewToolMessage(batch.ModelResults...))
	}
	reduced.resultPublication = &batch.Publication
	return reduced, nil
}
