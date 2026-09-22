package agentexec

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
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
	invocation, attempt, callID, err := o.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer o.session.accounting.discardPreparedModelContext(invocation)
	if beginExternalCallErr := attempt.beginExternalCall(); beginExternalCallErr != nil {
		return nil, beginExternalCallErr
	}
	response, err := o.model.Call(ctx, request)
	if err != nil {
		return response, o.finishFailedCall(ctx, invocation, attempt, callID, runs.ModelObservation{}, nil, err)
	}
	if response == nil {
		return nil, o.finishFailedCall(ctx, invocation, attempt, callID, runs.ModelObservation{}, nil, errors.New("agentexec: model returned no response"))
	}
	if err := response.Validate(); err != nil {
		return response, o.finishFailedCall(ctx, invocation, attempt, callID, runs.ModelObservation{}, nil, err)
	}
	if err := o.complete(ctx, invocation, callID, response, nil); err != nil {
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
		invocation, attempt, callID, err := o.begin(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		defer o.session.accounting.discardPreparedModelContext(invocation)
		if err := attempt.beginExternalCall(); err != nil {
			yield(nil, err)
			return
		}
		var accumulated corechat.ResponseAccumulator
		var text, reasoning strings.Builder
		observation := func() runs.ModelObservation {
			return runs.ModelObservation{Text: text.String(), Reasoning: reasoning.String()}
		}
		var firstOutputLatencyMillis *int64
		dispatchedAt := time.Now()
		for chunk, streamErr := range o.streamer.Stream(ctx, request) {
			if streamErr != nil {
				yield(nil, o.finishFailedCall(ctx, invocation, attempt, callID, observation(), firstOutputLatencyMillis, streamErr))
				return
			}
			receivedAt := time.Now()
			if err := accumulated.Add(chunk); err != nil {
				yield(nil, o.finishFailedCall(ctx, invocation, attempt, callID, observation(), firstOutputLatencyMillis, err))
				return
			}
			for _, part := range chunk.Parts {
				switch part.Kind {
				case corechat.PartDeltaText, corechat.PartDeltaRefusal:
					text.WriteString(part.Text)
				case corechat.PartDeltaReasoning:
					reasoning.WriteString(part.Text)
				}
			}
			if firstOutputLatencyMillis == nil && hasModelOutput(chunk) {
				firstOutputLatencyMillis = new(receivedAt.Sub(dispatchedAt).Milliseconds())
			}
			if !yield(chunk, nil) {
				if err := o.fail(ctx, invocation, callID, observation(), firstOutputLatencyMillis); err != nil {
					attempt.recordProjectionFailure(err)
				}
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
				observation(),
				firstOutputLatencyMillis,
				responseErr,
			))
			return
		}
		if err := response.Validate(); err != nil {
			yield(nil, o.finishFailedCall(ctx, invocation, attempt, callID, observation(), firstOutputLatencyMillis, err))
			return
		}
		if err := o.complete(ctx, invocation, callID, response, firstOutputLatencyMillis); err != nil {
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
	observation runs.ModelObservation,
	firstOutputLatencyMillis *int64,
	cause error,
) error {
	projectionErr := o.fail(ctx, invocation, callID, observation, firstOutputLatencyMillis)
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
	observation runs.ModelObservation,
	firstOutputLatencyMillis *int64,
) error {
	projectionCtx, cancel := o.session.lifetime.publicationContext(ctx)
	defer cancel()
	if err := o.session.flushDeltas(projectionCtx); err != nil {
		return err
	}
	return o.session.commitFact(
		projectionCtx,
		o.session.executorMember(invocation.Relation()),
		runs.ModelCallFailed{CallID: callID, Observation: observation, FirstOutputLatencyMillis: firstOutputLatencyMillis},
	)
}

func (o *observedInteractionModel) begin(
	ctx context.Context,
) (
	invocation interaction.ModelInvocation,
	attempt *dispatchAttempt,
	callID string,
	err error,
) {
	invocation, ok := interaction.ModelInvocationFromContext(ctx)
	if !ok {
		return interaction.ModelInvocation{}, nil, "", errors.New("agentexec: model call has no Interaction attribution")
	}
	preparedInvocation := invocation
	defer func() {
		if err != nil {
			o.session.accounting.discardPreparedModelContext(preparedInvocation)
			o.session.modelFailures.record(preparedInvocation.Relation().ProcessID(), interaction.HostFailure(err))
			err = errors.Join(err, o.session.stopModelProcess(ctx, preparedInvocation.Relation().ProcessID()))
		}
	}()
	attempt, err = dispatchAttemptFrom(ctx, invocation.EffectID())
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", err
	}
	callIdentity, err := modelInvocationID(invocation)
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", err
	}
	callID = callIdentity.String()
	member := o.session.executorMember(invocation.Relation())
	if err := o.session.commitAppliedInputs(
		ctx, member, invocation.Relation().ProcessID(), invocation.AppliedSteerSignalIDs(),
	); err != nil {
		return interaction.ModelInvocation{}, nil, "", interaction.HostFailure(err)
	}
	if err := o.session.commitFact(ctx, member, runs.ModelCallStarted{CallID: callID}); err != nil {
		return interaction.ModelInvocation{}, nil, "", interaction.HostFailure(
			fmt.Errorf("agentexec: commit model call start: %w", err),
		)
	}
	return invocation, attempt, callID, nil
}

func (o *observedInteractionModel) complete(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
	firstOutputLatencyMillis *int64,
) error {
	modelOutput := response.Output
	if modelOutput == nil || modelOutput.Message == nil {
		return errors.New("agentexec: completed model call has no assistant message")
	}
	// Agent owns Delta validation, ordering, buffering, and listener observation. Wait on its
	// ordering barrier before committing the authoritative full response so an
	// accepted stream increment can never reopen an Item after completion.
	projectionCtx, cancel := o.session.lifetime.publicationContext(ctx)
	defer cancel()
	if err := o.session.flushDeltas(projectionCtx); err != nil {
		return err
	}
	fact, err := o.session.accounting.accountModelCall(invocation, callID, response)
	if err != nil {
		return err
	}
	fact.FirstOutputLatencyMillis = firstOutputLatencyMillis
	if err := o.session.commitFact(
		projectionCtx, o.session.executorMember(invocation.Relation()), fact,
	); err != nil {
		return err
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

// Called only after accumulator validation. Opaque reasoning state and citation
// attachments are replay/annotation data, not the first generated output.
func hasModelOutput(delta *corechat.ResponseDelta) bool {
	for _, part := range delta.Parts {
		switch part.Kind {
		case corechat.PartDeltaText, corechat.PartDeltaRefusal, corechat.PartDeltaMedia, corechat.PartDeltaToolCall:
			return true
		case corechat.PartDeltaReasoning:
			if part.Text != "" {
				return true
			}
		}
	}
	return false
}
