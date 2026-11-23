package runs

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

const reasoningStream = "reasoning"

var (
	errExecutorContract = errors.New("runs: executor contract violation")
	errReducerInvariant = errors.New("runs: reducer invariant violation")
)

type reducerConfig struct {
	Opened            run.Run
	WorkspaceCWD      string
	Isolated          bool
	UserInput         []transcript.ContentBlock
	ConversationInput *corechat.Message
	ModelOnlyInput    bool
	Continuation      *treeContinuation
	Now               func() time.Time
}

type modelCall struct {
	startedAt time.Time
	purpose   run.ModelCallPurpose
}

type reducer struct {
	cfg                  reducerConfig
	resume               *resumeBinding
	itemIDs              segmentItemIdentities
	accrued              run.Metrics
	contextTokens        int64
	segmentDuration      time.Duration
	userInput            []transcript.ContentBlock
	text                 *openText
	reasoning            *openText
	modelCalls           map[string]modelCall
	modelBoundaryClosed  bool
	toolCallIDs          map[string]struct{}
	toolPositions        map[toolPosition]string
	restoredToolAttempts map[string]OpenToolInvocation
	tools                openTools
	drained              []DrainedTool
	errFailure           *run.Failure
	plan                 *PlanSnapshot
}

type openTool struct {
	callID            string
	sourceCallID      string
	modelCallSequence uint64
	toolCallIndex     uint32
	item              transcript.Item
	attemptInvocation transcript.ToolInvocation
	attemptStartedAt  time.Time
	finishedAt        time.Time
}

func (t *openTool) startedAs(start ToolCallStarted) bool {
	call, _ := t.item.ToolCall()
	return t.modelCallSequence == start.ModelCallSequence && t.toolCallIndex == start.ToolCallIndex &&
		t.sourceCallID == start.SourceCallID && call.Invocation.MatchesSource(start.ToolName, start.Source) &&
		t.attemptInvocation.Input.Equal(start.Input)
}

type toolPosition struct {
	modelCallSequence uint64
	toolCallIndex     uint32
}

func newReducer(cfg reducerConfig) *reducer {
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
		accrued:   cfg.Opened.Metrics(), contextTokens: cfg.Opened.ContextTokens(),
		modelCalls: make(map[string]modelCall), toolCallIDs: make(map[string]struct{}),
		toolPositions: make(map[toolPosition]string), tools: newOpenTools(),
	}
}

// The pump swaps this in only after the batch succeeds, so a rejected write cannot
// consume model/tool state or mint identities the durable projection never saw.
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
	cloned.restoredToolAttempts = maps.Clone(r.restoredToolAttempts)
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
	cloned.errFailure = r.errFailure
	if cloned.errFailure != nil {
		cloned.errFailure = new(*cloned.errFailure)
	}
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

func (r *reducer) nextItemID() (identity.ItemID, error) { return r.itemIDs.Next() }

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
		return r.streamDelta("text", r.appendText, e.Text)
	case ReasoningDelta:
		return r.streamDelta(reasoningStream, r.appendReasoning, e.Text)
	case ModelCallStarted:
		return r.startModelCall(e)
	case ModelCallCompleted:
		return r.completeModelCall(e)
	case ModelCallFailed:
		return r.failModelCall(e)
	case ToolCallStarted:
		if attempt, restored := r.restoredToolAttempts[e.CallID]; restored {
			return factReduction{}, r.adoptToolAttempt(e, attempt)
		}
		return r.startToolCall(e)
	case ToolCallPrepared:
		return r.prepareToolCall(e)
	case ToolResultsCommitted:
		return r.finishToolResults(e)
	case SteerMessagesApplied:
		return r.applySteers(e)
	case PlanUpdated:
		return factReduction{events: r.planSnapshot(e)}, nil
	case CompactionBoundary:
		events, err := r.compaction(e)
		if err != nil {
			err = fmt.Errorf("%w: compaction: %w", errReducerInvariant, err)
		}
		return factReduction{events: events}, err
	case SegmentEnded:
		return r.endSegment(e)
	default:
		return factReduction{}, fmt.Errorf("%w: unhandled event %T", errExecutorContract, ev)
	}
}

// Reasoning and message are concurrent projections of one response; completing one
// because the other emitted would duplicate it.
func (r *reducer) streamDelta(kind string, appendDelta func(string) ([]ProjectionEvent, error), text string) (factReduction, error) {
	if r.modelBoundaryClosed {
		return factReduction{}, nil
	}
	appended, err := appendDelta(text)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: append %s: %w", errReducerInvariant, kind, err)
	}
	return factReduction{events: appended}, nil
}

func (r *reducer) prepareToolCall(prepared ToolCallPrepared) (factReduction, error) {
	if err := prepared.Caller.Validate(); err != nil {
		return factReduction{}, err
	}
	caller := prepared.Caller.Snapshot()
	if caller.Invocation.ID != prepared.Start.CallID || !prepared.Start.MatchesCapturedRef(caller.Tool) {
		return factReduction{}, fmt.Errorf("%w: prepared Tool caller differs from execution", errExecutorContract)
	}
	prepared.Start.Source = caller.Tool
	reduced, err := r.startToolCall(prepared.Start)
	if err != nil {
		return reduced, err
	}
	if len(reduced.toolInvocations) != 1 {
		return factReduction{}, fmt.Errorf("%w: prepared Tool has no invocation", errExecutorContract)
	}
	reduced.toolInvocations[0].Caller = new(prepared.Caller)
	reduced.executionContext = new(prepared.Context.Clone())
	return reduced, nil
}

func (r *reducer) applySteers(applied SteerMessagesApplied) (factReduction, error) {
	events, err := r.steerMessagesApplied(applied)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: applied steers: %w", errExecutorContract, err)
	}
	if !r.cfg.Opened.Lineage().IsRoot() {
		return factReduction{events: events}, nil
	}
	messages := make([]corechat.Message, len(applied.Messages))
	for index, steer := range applied.Messages {
		message, err := MaterializeUserMessage(steer.Content)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: applied steer message[%d]: %w", errExecutorContract, index, err)
		}
		messages[index] = message
	}
	return factReduction{events: events, conversationMessages: messages}, nil
}

func (r *reducer) adoptToolAttempt(started ToolCallStarted, attempt OpenToolInvocation) error {
	if r.resume == nil || attempt.CallID != started.CallID || r.resume.callItems[started.CallID].ID() != attempt.ItemID ||
		attempt.SessionID != r.cfg.Opened.SessionID() || started.ModelCallSequence == 0 {
		return fmt.Errorf("%w: restored Tool attempt %q differs from its durable Item", errExecutorContract, started.CallID)
	}
	if _, err := r.toolStart(started); err != nil {
		return err
	}
	ref, _ := r.tools.get(started.CallID)
	ref.attemptStartedAt = attempt.StartedAt
	delete(r.restoredToolAttempts, started.CallID)
	return nil
}

func (r *reducer) startModelCall(started ModelCallStarted) (factReduction, error) {
	if err := started.Purpose.Validate(); err != nil {
		return factReduction{}, fmt.Errorf("%w: %w", errExecutorContract, err)
	}
	if err := ValidateInvocation(started.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call start: %w", errExecutorContract, err)
	}
	if _, duplicate := r.modelCalls[started.CallID]; duplicate {
		return factReduction{}, fmt.Errorf("%w: model call %q started more than once", errExecutorContract, started.CallID)
	}
	startedAt := r.now()
	r.modelCalls[started.CallID] = modelCall{startedAt: startedAt, purpose: started.Purpose}
	if started.Purpose == run.ModelConversation {
		r.modelBoundaryClosed = false
	}
	return factReduction{
		events: []ProjectionEvent{SegmentProgressed{Progress: Progress{Activity: CallingModelActivity{}}}},
		modelInvocations: []ModelInvocationCommit{{
			CallID: started.CallID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: modelref.InvocationStarted, Purpose: started.Purpose, StartedAt: startedAt,
		}},
	}, nil
}

func (r *reducer) completeModelCall(completed ModelCallCompleted) (factReduction, error) {
	if err := ValidateInvocation(completed.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call completion: %w", errExecutorContract, err)
	}
	call, started := r.modelCalls[completed.CallID]
	startedAt := call.startedAt
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
	if call.purpose == run.ModelIndependent && (completed.Message != nil || completed.ContextTokens != 0 || completed.FirstOutputLatencyMillis != nil) {
		return factReduction{}, fmt.Errorf("%w: independent model completion carries conversation state", errExecutorContract)
	}
	delete(r.modelCalls, completed.CallID)
	var events []ProjectionEvent
	var conversationMessages []corechat.Message
	if completed.Message != nil {
		message := *completed.Message
		var err error
		events, err = r.completeModelMessage(message)
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: model call completion: %w", errExecutorContract, err)
		}
		conversationMessages = r.rootConversationMessages(message)
	}
	if call.purpose == run.ModelConversation {
		r.modelBoundaryClosed = true
		previews, err := r.retainIncompleteModelOutput(ModelObservation{})
		if err != nil {
			return factReduction{}, fmt.Errorf("%w: retain unsettled model output: %w", errReducerInvariant, err)
		}
		events = append(events, previews...)
	}
	progressEvents, err := r.modelUsageProgress(completed)
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
			State: modelref.InvocationCompleted, Purpose: call.purpose, StartedAt: startedAt, FinishedAt: finishedAt, Usage: completed.ReportedUsage,
			FirstOutputLatencyMillis: completed.FirstOutputLatencyMillis,
		}},
		progress: &progressed,
	}, nil
}

func (r *reducer) failModelCall(failed ModelCallFailed) (factReduction, error) {
	if err := ValidateInvocation(failed.CallID); err != nil {
		return factReduction{}, fmt.Errorf("%w: model call failure: %w", errExecutorContract, err)
	}
	call, started := r.modelCalls[failed.CallID]
	if !started {
		return factReduction{}, fmt.Errorf("%w: model call %q failed without a start", errExecutorContract, failed.CallID)
	}
	finishedAt := r.now()
	if finishedAt.Before(call.startedAt) {
		return factReduction{}, fmt.Errorf("%w: model call %q failure precedes its start", errExecutorContract, failed.CallID)
	}
	if err := failed.validateFor(call.purpose); err != nil {
		return factReduction{}, err
	}
	var events []ProjectionEvent
	if call.purpose == run.ModelConversation {
		retained, err := r.retainIncompleteModelOutput(failed.Observation)
		if err != nil {
			return factReduction{}, err
		}
		events = retained
	}
	var usage *corechat.Usage
	var progressed *run.Run
	if failed.Accounting != nil {
		progressEvents, err := r.modelUsageProgress(*failed.Accounting)
		if err != nil {
			return factReduction{}, err
		}
		events = append(events, progressEvents...)
		value, err := r.progressedRun(finishedAt)
		if err != nil {
			return factReduction{}, err
		}
		progressed = &value
		usage = failed.Accounting.ReportedUsage
	}

	delete(r.modelCalls, failed.CallID)
	if call.purpose == run.ModelConversation {
		r.modelBoundaryClosed = true
	}
	return factReduction{
		progress: progressed,
		events:   append(events, SegmentProgressed{Progress: Progress{Activity: FailedModelCallActivity{}}}),
		modelInvocations: []ModelInvocationCommit{{
			CallID: failed.CallID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: modelref.InvocationFailed, Purpose: call.purpose, StartedAt: call.startedAt, FinishedAt: finishedAt,
			Usage: usage, KnownResponse: failed.KnownResponse.Clone(),
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
	if _, lost := ended.Ending.(run.Lost); len(r.modelCalls) > 0 && !lost {
		return factReduction{}, fmt.Errorf(
			"%w: segment ended with %d unsettled model calls",
			errExecutorContract,
			len(r.modelCalls),
		)
	}
	modelInvocations, err := r.closeLostModelCalls(ended.Ending)
	if err != nil {
		return factReduction{}, fmt.Errorf("%w: %w", errExecutorContract, err)
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

func (r *reducer) closeLostModelCalls(ending run.Ending) ([]ModelInvocationCommit, error) {
	if _, lost := ending.(run.Lost); !lost {
		return nil, nil
	}
	finishedAt := r.now()
	callIDs := slices.Sorted(maps.Keys(r.modelCalls))
	invocations := make([]ModelInvocationCommit, 0, len(callIDs))
	for _, callID := range callIDs {
		call := r.modelCalls[callID]
		startedAt := call.startedAt
		if finishedAt.Before(startedAt) {
			return nil, fmt.Errorf("model call %q loss precedes its start", callID)
		}
		invocations = append(invocations, ModelInvocationCommit{
			CallID: callID, SegmentID: r.cfg.Opened.ActiveSegmentID(),
			State: modelref.InvocationUnknown, Purpose: call.purpose, StartedAt: startedAt, FinishedAt: finishedAt,
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

func closedToolInvocationCommits(segmentID identity.SegmentID, tools []*openTool) []ToolInvocationCommit {
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

func (r *reducer) synthesizeTerminal(rootCancellationReason string) (reductionBatch, error) {
	out, err := r.retainIncompleteModelOutput(ModelObservation{})
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: retain unsettled model output: %w", errReducerInvariant, err)
	}
	openTools := r.tools.ordered()
	drained, err := r.drainTools()
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: drain tools: %w", errReducerInvariant, err)
	}
	out = append(out, drained...)
	var ending run.Ending = run.Canceled{Detail: rootCancellationReason}
	if len(r.modelCalls) > 0 {
		ending = run.Lost{Loss: run.Loss{Reason: run.LossUnresolvedModelCall}}
	} else if r.errFailure != nil {
		ending = run.Failed{Failure: *r.errFailure}
	}
	modelInvocations, err := r.closeLostModelCalls(ending)
	if err != nil {
		return reductionBatch{}, fmt.Errorf("%w: %w", errReducerInvariant, err)
	}
	terminal, err := r.finishedRun(ending, nil)
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

func (r *reducer) abort(cause error) {
	if r.errFailure == nil {
		r.errFailure = &run.Failure{Kind: run.FailureInternal, Detail: run.Diagnostic(cause)}
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
	if err := batch.validate(); err != nil {
		return factReduction{}, err
	}
	var reduced factReduction
	for _, start := range batch.Starts {
		if ref, open := r.tools.get(start.CallID); open {
			if !ref.startedAs(start) {
				return factReduction{}, errors.New("runs: result publication differs from its open call")
			}
			continue
		}
		started, err := r.startToolCall(start)
		if err != nil {
			return factReduction{}, err
		}
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
