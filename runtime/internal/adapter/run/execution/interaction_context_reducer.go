package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

const rootOpeningMessageCount = 1

type interactionModelContextReducer struct {
	compactor    ModelContextCompactor
	state        InteractionModelContextState
	session      *interactionSession
	start        runs.RootExecutionStart
	instructions []corechat.Message
	// catalog is the layer's frozen deferred Tool catalog. With the current
	// Session state it forms each call's tail, which the model boundary appends
	// after reduction: measured with the request, never compacted or returned.
	catalog []corechat.Message
	counter ModelContextInputTokenCounter
}

func newInteractionModelContextReducer(
	compactor ModelContextCompactor,
	state InteractionModelContextState,
	session *interactionSession,
	start runs.RootExecutionStart,
	instructions []corechat.Message,
	catalog []corechat.Message,
	counter ModelContextInputTokenCounter,
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

// ReduceModelContext builds the complete messages for one model call. The
// Dispatcher hands in a throwaway Clone and clones whatever comes back, so this
// reducer reslices and appends to request.Messages directly; copying it again
// would give the same sequence a second owner. The frozen instructions are this
// Runtime's own and are still copied before anything appends to them.
func (i *interactionModelContextReducer) ReduceModelContext(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	request *corechat.Request,
) ([]corechat.Message, error) {
	if i == nil || i.session == nil || request == nil || !invocation.Valid() {
		return nil, errors.New("execution: model-context reduction requires an attributed Interaction request")
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
	if i.compactor == nil {
		effective, err := appendPendingContinuation(
			request.Messages, pendingContinuation, hasPendingContinuation,
		)
		if err != nil {
			return nil, err
		}
		validation := request.Clone()
		validation.Messages = effective
		if err := validation.Validate(); err != nil {
			return nil, fmt.Errorf("execution: reduced model context: %w", err)
		}
		if err := i.session.tails.prepare(invocation, tail); err != nil {
			return nil, err
		}
		return effective, nil
	}
	candidate, err := appendPendingContinuation(
		request.Messages[len(i.instructions):], pendingContinuation, hasPendingContinuation,
	)
	if err != nil {
		return nil, err
	}
	fixedContext := cloneChatMessages(i.instructions)
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
		Instructions: fixedContext,
		Candidate:    candidate,
		Trailer:      tail,
		Tools:        request.Tools,
		Options:      request.Options,
		Calibration:  calibration,
		Counter:      i.counter,
		PreCompact:   preCompact,
	}
	var (
		compaction ModelContextCompaction
		buildErr   error
	)
	if invocation.Relation().IsRoot() {
		protectedTail := 0
		if invocation.ModelCallSequence() == 1 {
			protectedTail = rootOpeningMessageCount
		}
		if hasPendingContinuation {
			protectedTail = max(protectedTail, trailingUserMessageCount(candidate))
		}
		input.ProtectedTail = protectedTail
		compaction, buildErr = NewDurableModelContextCompaction(input)
	} else {
		protectedTail := 0
		if invocation.ModelCallSequence() == 1 {
			protectedTail = len(candidate)
		} else if signalCount := len(invocation.AppliedSteerSignalIDs()); signalCount > 0 {
			protectedTail = trailingUserMessageCount(candidate)
			if protectedTail < signalCount {
				return nil, fmt.Errorf(
					"execution: delegated model context attributes %d steer Signals but has only %d trailing User messages",
					signalCount,
					protectedTail,
				)
			}
		}
		input.ProtectedTail = protectedTail
		compaction, buildErr = NewTransientModelContextCompaction(input)
	}
	if buildErr != nil {
		return nil, buildErr
	}
	result, err := i.compactor.CompactModelContext(ctx, compaction)
	if err != nil {
		return nil, err
	}
	effective := append(
		fixedContext,
		result.Messages()...,
	)
	validation := request.Clone()
	validation.Messages = effective
	if err := validation.Validate(); err != nil {
		return nil, fmt.Errorf("execution: compacted model context: %w", err)
	}
	if err := i.session.accounting.prepareModelContext(invocation, result.EstimatedTokens()); err != nil {
		return nil, err
	}
	if err := i.session.tails.prepare(invocation, tail); err != nil {
		i.session.accounting.discardPreparedModelContext(invocation)
		return nil, err
	}
	if result.Summarized() {
		if compaction.Durable() {
			i.session.state.markDurableContextCompacted()
		}
		before, after := result.MessageCounts()
		i.session.lifetime.send(runs.ExecutorEvent{
			Member: i.session.executorMember(invocation.Relation()),
			Payload: runs.CompactionBoundary{
				Summary:        result.Summary(),
				MessagesBefore: before,
				MessagesAfter:  after,
			},
		})
	}
	return effective, nil
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
