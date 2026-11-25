package runs

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

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
