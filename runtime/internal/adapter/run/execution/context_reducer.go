package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	coreinvocation "github.com/Tangerg/scope/core/invocation"
)

const rootOpeningMessageCount = 1

type interactionModelContextReducer struct {
	compactor    ModelContextCompactor
	state        interactionModelContextState
	session      *interactionSession
	start        runs.RootExecutionStart
	instructions []corechat.Message
	// catalog is the layer's frozen deferred Tool catalog. With the current
	// Session state it forms each call's tail, which the model boundary appends
	// after reduction: measured with the request, never compacted or returned.
	catalog []corechat.Message
	counter modelContextInputTokenCounter
}

func newInteractionModelContextReducer(
	compactor ModelContextCompactor,
	state interactionModelContextState,
	session *interactionSession,
	start runs.RootExecutionStart,
	instructions []corechat.Message,
	catalog []corechat.Message,
	counter modelContextInputTokenCounter,
) *interactionModelContextReducer {
	return &interactionModelContextReducer{
		compactor:    compactor,
		state:        state,
		session:      session,
		start:        start,
		instructions: cloneChatMessages(instructions),
		catalog:      cloneChatMessages(catalog),
		counter:      counter,
	}
}

// appendPendingContinuation applies Runtime-owned input that already has a
// durable Item and so cannot arrive through the framework's input Signal.
func appendPendingContinuation(
	messages []corechat.Message,
	pending pendingInteractionContinuation,
	present bool,
) ([]corechat.Message, error) {
	if !present {
		return messages, nil
	}
	message, err := runs.MaterializeUserMessage(pending.content)
	if err != nil {
		return nil, fmt.Errorf("execution: materialize pending continuation input: %w", err)
	}
	return append(messages, message), nil
}

// PrepareContext returns only the authored context Scope may retain. The full
// provider request is captured separately and commits with this preparation.
func (i *interactionModelContextReducer) PrepareContext(
	ctx context.Context,
	preparation interaction.ContextInvocation,
	request *corechat.Request,
) (messages []corechat.Message, err error) {
	invocation := modelCallSource{process: preparation.Effect.ID().Process, effect: preparation.Effect.ID(),
		program: preparation.Effect.ProgramRef(), sequence: preparation.Sequence}
	if i == nil || i.session == nil || request == nil || !invocation.valid() {
		return nil, errors.New("execution: model-context reduction requires an attributed Interaction request")
	}
	if err := i.session.awaitDispatchSegment(ctx); err != nil {
		return nil, err
	}
	if err := i.session.commitRejectedToolFeedback(ctx, invocation, request); err != nil {
		return nil, err
	}
	request = request.Clone()
	request.Messages = withoutPreparationReference(request.Messages)
	invocation.steers, err = i.session.appliedSteers(request.Messages)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			i.session.effectFailures.record(invocation.effect, err)
			i.session.lifetime.wakeUnknown()
			return
		}
		if len(messages) == 0 {
			err = &interaction.ContextFailure{Cause: errors.New("execution: context preparation returned no messages")}
			return
		}
		err = messages[len(messages)-1].Metadata.Set(preparationMetadataKey, preparedModelReference{
			Process: invocation.process, Effect: invocation.effect, Sequence: invocation.sequence,
		})
	}()
	if err := i.validateContextSources(ctx, request); err != nil {
		return nil, err
	}
	calling, contextView, candidate, hasPendingContinuation, err := i.prepareContextView(ctx, invocation, request)
	if err != nil {
		return nil, err
	}
	if i.compactor == nil {
		validation, err := i.admitContext(calling, invocation, contextView, candidate)
		if err != nil {
			return nil, err
		}
		if err := i.session.material.prepare(invocation, validation); err != nil {
			return nil, err
		}
		return contextView.canonical(candidate), nil
	}
	return i.compactContext(calling, invocation, contextView, candidate, hasPendingContinuation)
}

func (i *interactionModelContextReducer) validateContextSources(ctx context.Context, request *corechat.Request) error {
	if i.session.callComposition != nil {
		bindings := i.session.compositionBindings()
		activations, err := i.session.callComposition.Activations(ctx, bindings)
		if err != nil {
			return err
		}
		if !slices.EqualFunc(activations, i.session.compositionActivations, plugin.Activation.Equal) {
			return plugin.ErrStale
		}
	}
	prefixMatches, err := sameInteractionMessages(
		request.Messages[:min(len(request.Messages), len(i.instructions))],
		i.instructions,
	)
	if err != nil {
		return err
	}
	if len(request.Messages) <= len(i.instructions) || !prefixMatches {
		return fmt.Errorf(
			"execution: model context no longer starts with its frozen instructions (messages=%d instructions=%d prefix_matches=%t)",
			len(request.Messages),
			len(i.instructions),
			prefixMatches,
		)
	}
	return nil
}

func (i *interactionModelContextReducer) prepareContextView(ctx context.Context, invocation modelCallSource, request *corechat.Request) (context.Context, modelRequestContext, []corechat.Message, bool, error) {
	pendingContinuation, hasPendingContinuation, err := i.session.pendingContinuationFor(
		invocation.process,
	)
	if err != nil {
		return nil, modelRequestContext{}, nil, false, err
	}
	tail, err := i.tail(ctx)
	if err != nil {
		return nil, modelRequestContext{}, nil, false, err
	}
	// One attributed context binds this reduction's auxiliary model work to the
	// executing member; context contribution, compaction, and input contribution
	// all inherit it so every physical call they dispatch is owned by this Run,
	// not re-derived (or dropped) per branch.
	identity, err := modelInvocationID(invocation)
	if err != nil {
		return nil, modelRequestContext{}, nil, false, err
	}
	calling := executionctx.WithModelCalls(ctx, modelCalls{session: i.session, member: i.session.executorMember(invocation.process)})
	calling, err = pluginmcp.WithInvocation(calling, framemcp.Invocation{ID: identity.String(), Origin: extension.Automatic, Parent: effectInvocationParent(invocation.effect), Target: portable.Target{Scope: extension.ScopeCall, Reference: identity.String()}})
	if err != nil {
		return nil, modelRequestContext{}, nil, false, err
	}
	var material []corechat.Message
	if len(i.session.modelContexts) > 0 {
		candidate := request.Clone()
		candidate.Messages, err = appendPendingContinuation(candidate.Messages, pendingContinuation, hasPendingContinuation)
		if err != nil {
			return nil, modelRequestContext{}, nil, false, err
		}
		material, err = i.session.callComposition.PrepareContext(calling, i.session.modelContexts, i.start.ModelSelection, candidate, tail)
		if err != nil {
			return nil, modelRequestContext{}, nil, false, err
		}
	}
	candidate, err := appendPendingContinuation(request.Messages[len(i.instructions):], pendingContinuation, hasPendingContinuation)
	if err != nil {
		return nil, modelRequestContext{}, nil, false, err
	}
	contextView := modelRequestContext{request: request, instructions: i.instructions, fixed: append(cloneChatMessages(i.instructions), cloneChatMessages(material)...), tail: tail}
	return calling, contextView, candidate, hasPendingContinuation, nil
}

func (i *interactionModelContextReducer) compactContext(calling context.Context, invocation modelCallSource, contextView modelRequestContext, candidate []corechat.Message, hasPendingContinuation bool) ([]corechat.Message, error) {
	preCompact := func(ctx context.Context) (bool, error) {
		if i.session.lifecycleHooks == nil {
			return true, nil
		}
		return i.session.lifecycleHooks.BeforeCompaction(
			ctx,
			i.start.SessionID,
			i.start.WorkspaceCWD,
		)
	}
	calibration := i.session.accounting.modelContextCalibration(invocation)

	input := ModelContextCompactionInput{
		SessionID:    i.start.SessionID,
		Selection:    i.start.ModelSelection,
		Limits:       i.session.selectedChat.TokenLimits(),
		RequestFits:  i.session.selectedChat.RequestFits,
		Instructions: contextView.fixed,
		Candidate:    candidate,
		Trailer:      contextView.tail,
		Tools:        contextView.request.Tools,
		Options:      contextView.request.Options,
		Calibration:  calibration,
		Counter:      i.counter,
		PreCompact:   preCompact,
	}
	// Canonical history is not the provider request when input transformation is
	// selected. Its compaction uses the local projection; final counting follows
	// transformation and owns the actual provider request.
	if len(i.session.modelInputs) > 0 {
		input.Counter = nil
	} else {
		countable := contextView.compose(candidate)
		if err := i.session.selectedChat.AdmitInput(countable); err != nil {
			if !errors.Is(err, catalog.ErrUnsupportedInputModality) {
				return nil, err
			}
			input.Counter = nil
		}
	}
	compaction, err := invocationCompaction(invocation, input, hasPendingContinuation)
	if err != nil {
		return nil, err
	}
	result, err := i.compactor.CompactModelContext(calling, compaction)
	if err != nil {
		if kind := coreinvocation.ClassifyFailure(err); kind == coreinvocation.FailureFailed || kind == coreinvocation.FailureNotExecuted {
			err = &interaction.ContextFailure{Cause: err}
		}
		return nil, err
	}
	effective := contextView.canonical(result.Messages())
	validation, err := i.admitContext(calling, invocation, contextView, result.Messages())
	if err != nil {
		return nil, err
	}
	estimate := result.EstimatedTokens()
	if len(i.session.modelInputs) > 0 {
		estimate, err = model.EstimateRequestTokens(validation)
		if err != nil {
			return nil, err
		}
	}
	if err := i.session.accounting.prepareModelContext(invocation, estimate); err != nil {
		return nil, err
	}
	if err := i.session.material.prepare(invocation, validation); err != nil {
		i.session.accounting.discardPreparedModelContext(invocation)
		return nil, err
	}
	if err := i.commitCompaction(calling, invocation.process, compaction, result); err != nil {
		return nil, err
	}
	return effective, nil
}

func (i *interactionModelContextReducer) commitCompaction(ctx context.Context, process scoperun.ProcessID, compaction ModelContextCompaction, result ModelContextCompactionResult) error {
	if result.Changed() {
		summary := result.Summary()
		if summary == "" {
			summary = "Trimmed model context."
		}
		before, after := result.MessageCounts()
		fact := runs.CompactionBoundary{Summary: summary, MessagesBefore: before, MessagesAfter: after}
		if compaction.Durable() {
			rewrite, found := result.Rewrite()
			if !found {
				return errors.New("execution: durable compaction has no history proposal")
			}
			fact.Rewrite = new(rewrite)
		}
		if err := i.session.commitFact(ctx, i.session.executorMember(process), fact); err != nil {
			return err
		}
		if compaction.Durable() {
			i.session.state.markDurableContextCompacted()
		}
	}
	return nil
}

type modelRequestContext struct {
	request                   *corechat.Request
	instructions, fixed, tail []corechat.Message
}

func (c modelRequestContext) canonical(history []corechat.Message) []corechat.Message {
	return append(cloneChatMessages(c.instructions), history...)
}

func (c modelRequestContext) compose(history []corechat.Message) *corechat.Request {
	request := c.request.Clone()
	request.Messages = append(cloneChatMessages(c.fixed), cloneChatMessages(history)...)
	request.Messages = append(request.Messages, cloneChatMessages(c.tail)...)
	return request
}

func (i *interactionModelContextReducer) admitContext(ctx context.Context, invocation modelCallSource, view modelRequestContext, history []corechat.Message) (*corechat.Request, error) {
	request := view.compose(history)
	if err := request.Validate(); err != nil {
		return nil, &interaction.ContextFailure{Cause: fmt.Errorf("execution: reduced model context: %w", err)}
	}
	return i.prepareInput(ctx, invocation, request)
}

func invocationCompaction(invocation modelCallSource, input ModelContextCompactionInput, pending bool) (ModelContextCompaction, error) {
	if invocation.process.Depth() == 0 {
		if invocation.sequence == 1 {
			input.ProtectedTail = rootOpeningMessageCount
		}
		if pending {
			input.ProtectedTail = max(input.ProtectedTail, trailingUserMessageCount(input.Candidate))
		}
		return NewDurableModelContextCompaction(input)
	}
	if invocation.sequence == 1 {
		input.ProtectedTail = len(input.Candidate)
	} else if signals := len(invocation.steers); signals > 0 {
		input.ProtectedTail = trailingUserMessageCount(input.Candidate)
		if input.ProtectedTail < signals {
			return ModelContextCompaction{}, fmt.Errorf("execution: delegated model context attributes %d steer Signals but has only %d trailing User messages", signals, input.ProtectedTail)
		}
	}
	return newTransientModelContextCompaction(input)
}

// tail composes this call's Runtime-authored tail: the current Session state,
// read now from its owners, then the frozen deferred catalog. Neither is part
// of the context Scope adopts, so a Plan or Goal change never rewrites an
// earlier message of the cached prefix.
func (i *interactionModelContextReducer) tail(ctx context.Context) ([]corechat.Message, error) {
	var tail []corechat.Message
	if i.state != nil {
		state, err := i.state.CurrentSessionState(ctx, i.start.SessionID)
		if err != nil {
			return nil, err
		}
		tail = append(tail, state...)
	}
	return append(tail, cloneChatMessages(i.catalog)...), nil
}

func trailingUserMessageCount(messages []corechat.Message) int {
	count := 0
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role != corechat.RoleUser {
			break
		}
		count++
	}
	return count
}

func sameInteractionMessages(left, right []corechat.Message) (bool, error) {
	if len(left) != len(right) {
		return false, nil
	}
	// Compare Scope's complete canonical values, including citations and future
	// message fields, without making metadata whitespace part of identity.
	effective, err := scoperun.EncodePayload(left)
	if err != nil {
		return false, fmt.Errorf("execution: effective instructions: %w", err)
	}
	frozen, err := scoperun.EncodePayload(right)
	if err != nil {
		return false, fmt.Errorf("execution: frozen instructions: %w", err)
	}
	return bytes.Equal(effective.JSON(), frozen.JSON()), nil
}

var _ interaction.ContextProcessor = (*interactionModelContextReducer)(nil)

func (i *interactionModelContextReducer) prepareInput(calling context.Context, invocation modelCallSource, request *corechat.Request) (*corechat.Request, error) {
	if len(i.session.modelInputs) == 0 {
		if err := i.session.selectedChat.AdmitInput(request); err != nil {
			return nil, &interaction.ContextFailure{Cause: err}
		}
		return request, nil
	}
	transformed, err := i.session.callComposition.PrepareInput(calling, i.session.modelInputs, i.start.ModelSelection, request)
	if err != nil {
		if _, definite := err.(*framemcp.Refusal); definite {
			return nil, &interaction.ContextFailure{Cause: err}
		}
		return nil, err
	}
	if err := pluginsdk.ValidateModelInput(request, transformed); err != nil {
		return nil, &interaction.ContextFailure{Cause: err}
	}
	// Modality and request validation is local and proves refusal. The later
	// provider capacity measurement can perform I/O and retains its own outcome.
	if err := i.session.selectedChat.AdmitInput(transformed); err != nil {
		return nil, &interaction.ContextFailure{Cause: err}
	}
	if err := i.session.selectedChat.AdmitRequest(calling, transformed); err != nil {
		return nil, err
	}
	if i.compactor == nil {
		estimate, err := model.EstimateRequestTokens(transformed)
		if err != nil {
			return nil, err
		}
		if err := i.session.accounting.prepareModelContext(invocation, estimate); err != nil {
			return nil, err
		}
	}
	return transformed, nil
}
