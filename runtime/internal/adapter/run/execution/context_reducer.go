package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
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

// ReduceModelContext returns only the authored context Scope may retain. It
// measures contributed instructions and the current Session tail with that
// context, then transfers them once to the provider boundary.
func (i *interactionModelContextReducer) ReduceModelContext(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	request *corechat.Request,
) ([]corechat.Message, error) {
	if i == nil || i.session == nil || request == nil || !invocation.Valid() {
		return nil, errors.New("execution: model-context reduction requires an attributed Interaction request")
	}
	if i.session.callComposition != nil {
		bindings := i.session.compositionBindings()
		activations, err := i.session.callComposition.Activations(ctx, bindings)
		if err != nil {
			return nil, err
		}
		if !slices.EqualFunc(activations, i.session.compositionActivations, plugin.Activation.Equal) {
			return nil, plugin.ErrStale
		}
	}
	prefixMatches, err := sameInteractionMessages(
		request.Messages[:min(len(request.Messages), len(i.instructions))],
		i.instructions,
	)
	if err != nil {
		return nil, err
	}
	if len(request.Messages) <= len(i.instructions) || !prefixMatches {
		return nil, fmt.Errorf(
			"execution: model context no longer starts with its frozen instructions (messages=%d instructions=%d prefix_matches=%t)",
			len(request.Messages),
			len(i.instructions),
			prefixMatches,
		)
	}
	pendingContinuation, hasPendingContinuation, err := i.session.pendingContinuationFor(
		invocation.Relation().ProcessID(),
	)
	if err != nil {
		return nil, err
	}
	tail, err := i.tail(ctx)
	if err != nil {
		return nil, err
	}
	// One attributed context binds this reduction's auxiliary model work to the
	// executing member; context contribution, compaction, and input contribution
	// all inherit it so every physical call they dispatch is owned by this Run,
	// not re-derived (or dropped) per branch.
	identity, err := modelInvocationID(invocation)
	if err != nil {
		return nil, err
	}
	calling := executionctx.WithModelCalls(ctx, modelCalls{session: i.session, member: i.session.executorMember(invocation.Relation())})
	calling, err = qimumcp.WithInvocation(calling, portable.Invocation{ID: identity.String(), Origin: extension.Automatic, Parent: invocation.EffectID().String(), Target: portable.Target{Scope: extension.ScopeCall, Reference: identity.String()}})
	if err != nil {
		return nil, err
	}
	var material []corechat.Message
	if len(i.session.modelContexts) > 0 {
		candidate := request.Clone()
		candidate.Messages, err = appendPendingContinuation(candidate.Messages, pendingContinuation, hasPendingContinuation)
		if err != nil {
			return nil, err
		}
		material, err = i.session.callComposition.PrepareContext(calling, i.session.modelContexts, i.start.ModelSelection, candidate, tail)
		if err != nil {
			return nil, err
		}
	}
	candidate, err := appendPendingContinuation(request.Messages[len(i.instructions):], pendingContinuation, hasPendingContinuation)
	if err != nil {
		return nil, err
	}
	contextView := modelRequestContext{request: request, instructions: i.instructions, fixed: append(cloneChatMessages(i.instructions), cloneChatMessages(material)...), tail: tail}
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
		Trailer:      tail,
		Tools:        request.Tools,
		Options:      request.Options,
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
				return nil, errors.New("execution: durable compaction has no history proposal")
			}
			fact.Rewrite = new(rewrite)
		}
		if err := i.session.commitFact(ctx, i.session.executorMember(invocation.Relation()), fact); err != nil {
			return nil, err
		}
		if compaction.Durable() {
			i.session.state.markDurableContextCompacted()
		}
	}
	return effective, nil
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

func (i *interactionModelContextReducer) admitContext(ctx context.Context, invocation interaction.ModelInvocation, view modelRequestContext, history []corechat.Message) (*corechat.Request, error) {
	request := view.compose(history)
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("execution: reduced model context: %w", err)
	}
	return i.prepareInput(ctx, invocation, request)
}

func invocationCompaction(invocation interaction.ModelInvocation, input ModelContextCompactionInput, pending bool) (ModelContextCompaction, error) {
	if invocation.Relation().IsRoot() {
		if invocation.ModelCallSequence() == 1 {
			input.ProtectedTail = rootOpeningMessageCount
		}
		if pending {
			input.ProtectedTail = max(input.ProtectedTail, trailingUserMessageCount(input.Candidate))
		}
		return NewDurableModelContextCompaction(input)
	}
	if invocation.ModelCallSequence() == 1 {
		input.ProtectedTail = len(input.Candidate)
	} else if signals := len(invocation.AppliedSteerSignalIDs()); signals > 0 {
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
	effective, err := agent.EncodePayload(left)
	if err != nil {
		return false, fmt.Errorf("execution: effective instructions: %w", err)
	}
	frozen, err := agent.EncodePayload(right)
	if err != nil {
		return false, fmt.Errorf("execution: frozen instructions: %w", err)
	}
	return bytes.Equal(effective.JSON(), frozen.JSON()), nil
}

var _ interaction.ModelContextReducer = (*interactionModelContextReducer)(nil)

func (i *interactionModelContextReducer) prepareInput(calling context.Context, invocation interaction.ModelInvocation, request *corechat.Request) (*corechat.Request, error) {
	if len(i.session.modelInputs) == 0 {
		if err := i.session.selectedChat.AdmitInput(request); err != nil {
			return nil, err
		}
		return request, nil
	}
	transformed, err := i.session.callComposition.PrepareInput(calling, i.session.modelInputs, i.start.ModelSelection, request)
	if err != nil {
		return nil, err
	}
	if err := pluginsdk.ValidateModelInput(request, transformed); err != nil {
		return nil, err
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
