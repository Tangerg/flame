package agentexec

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type observedInteractionModel struct {
	inner   *chatclient.Client
	session *interactionSession
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
	response, err := o.inner.Call(ctx, request)
	if err != nil {
		if projectionErr := o.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(err, projectionErr)
		}
		o.session.modelFailures.record(invocation.Relation().ProcessID(), err)
		return response, err
	}
	if response == nil {
		responseErr := errors.New("agentexec: model returned no response")
		if projectionErr := o.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(responseErr, projectionErr)
		}
		o.session.modelFailures.record(invocation.Relation().ProcessID(), responseErr)
		return nil, responseErr
	}
	if err := response.Validate(); err != nil {
		if projectionErr := o.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(err, projectionErr)
		}
		o.session.modelFailures.record(invocation.Relation().ProcessID(), err)
		return response, err
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
		for chunk, streamErr := range o.inner.Stream(ctx, request) {
			if streamErr != nil {
				yield(nil, o.finishFailedStream(ctx, invocation, attempt, callID, streamErr))
				return
			}
			if err := accumulated.Add(chunk); err != nil {
				yield(nil, o.finishFailedStream(ctx, invocation, attempt, callID, err))
				return
			}
			if !yield(chunk, nil) {
				_ = o.finishFailedStream(ctx, invocation, attempt, callID, nil)
				return
			}
		}
		response, responseErr := accumulated.Response()
		if responseErr != nil {
			yield(nil, o.finishFailedStream(
				ctx,
				invocation,
				attempt,
				callID,
				responseErr,
			))
			return
		}
		if err := response.Validate(); err != nil {
			yield(nil, o.finishFailedStream(ctx, invocation, attempt, callID, err))
			return
		}
		if err := o.complete(ctx, invocation, callID, response); err != nil {
			attempt.recordProjectionFailure(err)
			yield(nil, err)
		}
	}
}

func (o *observedInteractionModel) finishFailedStream(
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
		return cause
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
	allowanceTurn, err = o.acquireAllowance(ctx)
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", nil, err
	}
	defer func() {
		if err != nil {
			allowanceTurn.release()
		}
	}()
	if _, err := o.session.reconcileCompletedDelegateChildren(ctx); err != nil {
		return interaction.ModelInvocation{}, nil, "", nil, interaction.HostFailure(err)
	}
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
	return invocation, attempt, callID, allowanceTurn, nil
}

func (o *observedInteractionModel) acquireAllowance(ctx context.Context) (*interactionAllowanceTurn, error) {
	turn, err := o.session.allowance.acquire(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := o.session.accounting.snapshot()
	if err != nil {
		turn.release()
		return nil, interaction.HostFailure(err)
	}
	if err := o.session.allowance.admit(usage); err != nil {
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

func newObservedInteractionClient(
	inner *chatclient.Client,
	session *interactionSession,
) (*chatclient.Client, error) {
	observed := &observedInteractionModel{inner: inner, session: session}
	client, err := chatclient.New(observed, chatclient.Config{Streamer: observed})
	if err != nil {
		return nil, err
	}
	return &client, nil
}

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
		cost = pricing(selection.Provider(), servedModel, &metadata.Usage)
	}
	return accounting.ModelUsage{
		Model: servedModel, TokenUsage: accountingTokenUsage(metadata.Usage), Cost: cost, Calls: 1,
	}
}

func accountingTokenUsage(usage corechat.Usage) accounting.TokenUsage {
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
