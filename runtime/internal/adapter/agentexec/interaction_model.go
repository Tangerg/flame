package agentexec

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

// observedInteractionModel projects one resolved provider instance as the
// Interaction model boundary. Call and Stream are separate capabilities of that
// instance, so a provider without streaming simply has no streamer here.
type observedInteractionModel struct {
	model    corechat.Model
	streamer corechat.Streamer
	session  *interactionSession
}

func (o *observedInteractionModel) Call(
	ctx context.Context,
	request *corechat.Request,
) (*corechat.Response, error) {
	invocation, attempt, callID, allowanceTurn, err := o.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer allowanceTurn.release()
	defer o.session.accounting.discardPreparedModelContext(invocation)
	if beginExternalCallErr := attempt.beginExternalCall(); beginExternalCallErr != nil {
		return nil, beginExternalCallErr
	}
	response, err := o.model.Call(ctx, request)
	if err != nil {
		return response, o.finishFailedCall(ctx, invocation, attempt, callID, err)
	}
	if response == nil {
		return nil, o.finishFailedCall(ctx, invocation, attempt, callID, errors.New("agentexec: model returned no response"))
	}
	if err := response.Validate(); err != nil {
		return response, o.finishFailedCall(ctx, invocation, attempt, callID, err)
	}
	if err := o.complete(ctx, invocation, callID, response); err != nil {
		attempt.recordProjectionFailure(err)
		return nil, err
	}
	return response, nil
}

func (o *observedInteractionModel) Stream(
	ctx context.Context,
	request *corechat.Request,
) iter.Seq2[*corechat.ResponseDelta, error] {
	return func(yield func(*corechat.ResponseDelta, error) bool) {
		invocation, attempt, callID, allowanceTurn, err := o.begin(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		defer allowanceTurn.release()
		defer o.session.accounting.discardPreparedModelContext(invocation)
		if err := attempt.beginExternalCall(); err != nil {
			yield(nil, err)
			return
		}
		var accumulated corechat.ResponseAccumulator
		for chunk, streamErr := range o.streamer.Stream(ctx, request) {
			if streamErr != nil {
				yield(nil, o.finishFailedCall(ctx, invocation, attempt, callID, streamErr))
				return
			}
			if err := accumulated.Add(chunk); err != nil {
				yield(nil, o.finishFailedCall(ctx, invocation, attempt, callID, err))
				return
			}
			if !yield(chunk, nil) {
				_ = o.finishFailedCall(ctx, invocation, attempt, callID, nil)
				return
			}
		}
		response, responseErr := accumulated.Response()
		if responseErr != nil {
			yield(nil, o.finishFailedCall(
				ctx,
				invocation,
				attempt,
				callID,
				responseErr,
			))
			return
		}
		if err := response.Validate(); err != nil {
			yield(nil, o.finishFailedCall(ctx, invocation, attempt, callID, err))
			return
		}
		if err := o.complete(ctx, invocation, callID, response); err != nil {
			attempt.recordProjectionFailure(err)
			yield(nil, err)
		}
	}
}

func (o *observedInteractionModel) finishFailedCall(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	attempt *dispatchAttempt,
	callID string,
	cause error,
) error {
	projectionErr := o.fail(ctx, invocation, callID)
	if projectionErr == nil {
		if cause != nil {
			o.session.modelFailures.record(invocation.Relation().ProcessID(), cause)
		}
		return errors.Join(cause, o.session.stopModelProcess(ctx, invocation.Relation().ProcessID()))
	}
	attempt.recordProjectionFailure(projectionErr)
	return errors.Join(cause, projectionErr)
}

func (o *observedInteractionModel) fail(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
) error {
	projectionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
	defer cancel()
	return o.session.commitFact(
		projectionCtx,
		o.session.executorMember(invocation.Relation()),
		runs.ModelCallFailed{CallID: callID},
	)
}

func (o *observedInteractionModel) begin(
	ctx context.Context,
) (
	invocation interaction.ModelInvocation,
	attempt *dispatchAttempt,
	callID string,
	allowanceTurn *interactionAllowanceTurn,
	err error,
) {
	invocation, ok := interaction.ModelInvocationFromContext(ctx)
	if !ok {
		return interaction.ModelInvocation{}, nil, "", nil, errors.New("agentexec: model call has no Interaction attribution")
	}
	preparedInvocation := invocation
	defer func() {
		if err != nil {
			o.session.accounting.discardPreparedModelContext(preparedInvocation)
			if !errors.Is(err, errInteractionAllowanceDenied) {
				o.session.modelFailures.record(preparedInvocation.Relation().ProcessID(), interaction.HostFailure(err))
			}
			err = errors.Join(err, o.session.stopModelProcess(ctx, preparedInvocation.Relation().ProcessID()))
		}
	}()
	attempt, err = dispatchAttemptFrom(ctx, invocation.EffectID())
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", nil, err
	}
	callIdentity, err := modelInvocationID(invocation)
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", nil, err
	}
	callID = callIdentity.String()
	// The turn is held in a local: a failure below returns a nil allowanceTurn
	// result, and a cleanup that read the named result would release nothing and
	// wedge the next model call on a finite Run.
	turn, err := o.acquireAllowance(ctx, invocation.Relation().ProcessID())
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", nil, err
	}
	defer func() {
		if err != nil {
			turn.release()
		}
	}()
	member := o.session.executorMember(invocation.Relation())
	if err := o.session.commitAppliedInputs(
		ctx, member, invocation.Relation().ProcessID(), invocation.AppliedSteerSignalIDs(),
	); err != nil {
		return interaction.ModelInvocation{}, nil, "", nil, interaction.HostFailure(err)
	}
	if err := o.session.commitFact(ctx, member, runs.ModelCallStarted{CallID: callID}); err != nil {
		return interaction.ModelInvocation{}, nil, "", nil, interaction.HostFailure(
			fmt.Errorf("agentexec: commit model call start: %w", err),
		)
	}
	return invocation, attempt, callID, turn, nil
}

func (o *observedInteractionModel) acquireAllowance(ctx context.Context, processID agent.ProcessID) (*interactionAllowanceTurn, error) {
	turn, err := o.session.allowance.acquire(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := o.session.accounting.snapshot()
	if err != nil {
		turn.release()
		return nil, interaction.HostFailure(err)
	}
	if err := o.session.allowance.admit(processID, usage); err != nil {
		turn.release()
		return nil, err
	}
	return turn, nil
}

func (o *observedInteractionModel) complete(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
) error {
	modelOutput := response.Output
	if modelOutput == nil || modelOutput.Message == nil {
		return errors.New("agentexec: completed model call has no assistant message")
	}
	// Agent owns Delta validation, ordering, buffering, and listener observation. Wait on its
	// ordering barrier before committing the authoritative full response so an
	// accepted stream increment can never reopen an Item after completion.
	if err := o.session.flushDeltas(ctx); err != nil {
		return err
	}
	fact, err := o.session.accounting.accountModelCall(invocation, callID, response)
	if err != nil {
		return err
	}
	projectionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
	defer cancel()
	if err := o.session.commitFact(
		projectionCtx, o.session.executorMember(invocation.Relation()), fact,
	); err != nil {
		return err
	}
	if !invocation.Relation().IsRoot() {
		o.session.committedReplies.record(invocation.Relation().ProcessID(), fact.Message)
	}
	return o.session.registerDelegateCalls(invocation, modelOutput.Message)
}

// newObservedInteractionModel wraps the resolved provider so every model call
// is attributed before it leaves the Runtime. Streaming is offered only when the
// provider itself streams; the Dispatcher takes exactly one of the two.
func newObservedInteractionModel(
	model corechat.Model,
	streamer corechat.Streamer,
	session *interactionSession,
) (*observedInteractionModel, error) {
	if dependency.Missing(model) {
		return nil, errors.New("agentexec: Interaction model is required")
	}
	return &observedInteractionModel{model: model, streamer: streamer, session: session}, nil
}

// Streams reports whether this boundary can serve the Dispatcher's streaming
// capability.
func (o *observedInteractionModel) Streams() bool { return !dependency.Missing(o.streamer) }

func modelUsage(
	response *corechat.Response,
	selection modelref.Selection,
	pricing accounting.Pricing,
) accounting.ModelUsage {
	var metadata corechat.ResponseMetadata
	if response.Metadata != nil {
		metadata = *response.Metadata
	}
	servedModel := metadata.Model
	if servedModel == "" {
		servedModel = selection.Model()
	}
	var cost accounting.Cost
	if pricing != nil {
		cost = pricing(selection.Provider(), servedModel, metadata.Usage)
	}
	return accounting.ModelUsage{
		Model: servedModel, TokenUsage: accountingTokenUsage(metadata.Usage), Cost: cost, Calls: 1,
	}
}

func accountingTokenUsage(usage *corechat.Usage) accounting.TokenUsage {
	if usage == nil {
		return accounting.TokenUsage{}
	}
	result := accounting.TokenUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
	}
	if usage.ReasoningTokens != nil {
		result.ReasoningTokens = *usage.ReasoningTokens
	}
	if usage.CacheReadInputTokens != nil {
		result.CacheReadTokens = *usage.CacheReadInputTokens
	}
	if usage.CacheWriteInputTokens != nil {
		result.CacheWriteTokens = *usage.CacheWriteInputTokens
	}
	return result
}
