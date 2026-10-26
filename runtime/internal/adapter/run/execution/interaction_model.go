package execution

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

// observedInteractionModel projects one resolved provider instance as the
// Interaction model boundary. Call and Stream are separate capabilities of that
// instance, so a provider without streaming simply has no streamer here.
type observedInteractionModel struct {
	model    corechat.Model
	streamer corechat.Streamer
	session  *interactionSession
}

// request attaches exactly the ephemeral material measured by the reducer after
// Scope has adopted the authored conversation it will retain.
func (o *observedInteractionModel) request(invocation interaction.ModelInvocation, request *corechat.Request) (*corechat.Request, error) {
	material, err := o.session.material.take(invocation)
	if err != nil {
		return nil, err
	}
	if request == nil || material.request == nil {
		return nil, errors.New("execution: prepared model request is unavailable")
	}
	return material.request, nil
}

func (o *observedInteractionModel) Call(
	ctx context.Context,
	request *corechat.Request,
) (*corechat.Response, error) {
	invocation, callID, err := o.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer o.session.accounting.discardPreparedModelContext(invocation)
	defer o.session.material.discard(invocation)
	sent, err := o.request(invocation, request)
	if err != nil {
		return nil, o.finishFailedCall(ctx, invocation, callID, runs.ModelObservation{}, nil, &corechat.CallFailure{Kind: corechat.CallFailureNotExecuted, Cause: err})
	}
	ctx = o.session.modelCallContext(ctx, o.session.executorMember(invocation.Relation()), callID)
	ctx, err = pluginsdk.WithInvocation(ctx, extension.Invocation{ID: callID, Origin: extension.Automatic, Parent: invocation.EffectID().String(), Target: extension.Target{Scope: extension.ScopeCall, Reference: callID}})
	if err != nil {
		return nil, o.finishFailedCall(ctx, invocation, callID, runs.ModelObservation{}, nil, &corechat.CallFailure{Kind: corechat.CallFailureNotExecuted, Cause: err})
	}
	response, err := o.model.Call(ctx, sent)
	if err != nil {
		return response, o.finishFailedCall(ctx, invocation, callID, runs.ModelObservation{}, nil, err)
	}
	if response == nil {
		return nil, o.finishFailedCall(ctx, invocation, callID, runs.ModelObservation{}, nil, errors.New("execution: model returned no response"))
	}
	if err := response.Validate(); err != nil {
		return response, o.finishFailedCall(ctx, invocation, callID, runs.ModelObservation{}, nil, err)
	}
	if err := o.complete(ctx, invocation, callID, response, nil); err != nil {
		return response, err
	}
	return response, nil
}

func (o *observedInteractionModel) Stream(
	ctx context.Context,
	request *corechat.Request,
) iter.Seq2[*corechat.ResponseDelta, error] {
	return func(yield func(*corechat.ResponseDelta, error) bool) {
		invocation, callID, err := o.begin(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		defer o.session.accounting.discardPreparedModelContext(invocation)
		defer o.session.material.discard(invocation)
		sent, err := o.request(invocation, request)
		if err != nil {
			yield(nil, o.finishFailedCall(ctx, invocation, callID, runs.ModelObservation{}, nil, &corechat.CallFailure{Kind: corechat.CallFailureNotExecuted, Cause: err}))
			return
		}
		dispatchedAt := time.Now()
		ctx = o.session.modelCallContext(ctx, o.session.executorMember(invocation.Relation()), callID)
		ctx, err = pluginsdk.WithInvocation(ctx, extension.Invocation{ID: callID, Origin: extension.Automatic, Parent: invocation.EffectID().String(), Target: extension.Target{Scope: extension.ScopeCall, Reference: callID}})
		if err != nil {
			yield(nil, o.finishFailedCall(ctx, invocation, callID, runs.ModelObservation{}, nil, &corechat.CallFailure{Kind: corechat.CallFailureNotExecuted, Cause: err}))
			return
		}
		sequence := o.streamer.Stream(ctx, sent)
		if sequence == nil {
			yield(nil, o.finishFailedCall(ctx, invocation, callID, runs.ModelObservation{}, nil,
				errors.New("execution: model streamer returned a nil sequence")))
			return
		}
		var accumulated corechat.ResponseAccumulator
		var text, reasoning strings.Builder
		observation := func() runs.ModelObservation {
			return runs.ModelObservation{Text: text.String(), Reasoning: reasoning.String()}
		}
		var firstOutputLatencyMillis *int64
		var failure error
		var stopped bool
		for chunk, streamErr := range sequence {
			if streamErr != nil {
				failure = streamErr
				break
			}
			receivedAt := time.Now()
			if err := accumulated.Add(chunk); err != nil {
				failure = err
				break
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
				stopped = true
				break
			}
		}
		// Every early exit must unwind the physical stream before reading its
		// settled evidence, including errors raised by an outer observer.
		if failure != nil {
			yield(nil, o.finishFailedCall(ctx, invocation, callID, observation(), firstOutputLatencyMillis, failure))
			return
		}
		if stopped {
			if err := o.settleFailedCall(ctx, invocation, callID, observation(), firstOutputLatencyMillis); err != nil {
				// Preserve the settlement failure locally; yielding again violates the stream.
				o.session.effectFailures.record(invocation.EffectID(), err)
			}
			return
		}
		response, responseErr := accumulated.Response()
		if responseErr != nil {
			yield(nil, o.finishFailedCall(
				ctx,
				invocation,
				callID,
				observation(),
				firstOutputLatencyMillis,
				responseErr,
			))
			return
		}
		if err := o.complete(ctx, invocation, callID, response, firstOutputLatencyMillis); err != nil {
			yield(nil, err)
		}
	}
}

func (o *observedInteractionModel) finishFailedCall(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
	observation runs.ModelObservation,
	firstOutputLatencyMillis *int64,
	cause error,
) error {
	projectionErr := o.settleFailedCall(ctx, invocation, callID, observation, firstOutputLatencyMillis)
	return o.stopFailedCall(ctx, invocation, cause, projectionErr)
}

func (o *observedInteractionModel) endpoint(ctx context.Context, callID string) (*modelref.EndpointObservation, error) {
	row, found, err := o.session.modelEvidence.EndpointForCaller(ctx, callID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	if row.Invocation.Caller.ID != callID || row.Invocation.Provider != o.session.start.ModelSelection.Provider() || row.Invocation.Model != o.session.start.ModelSelection.Model() {
		return nil, errors.New("execution: physical model evidence differs from captured call")
	}
	value := row.Clone()
	return &value, nil
}

func (o *observedInteractionModel) accountEndpoint(invocation interaction.ModelInvocation, callID string, source modelref.EndpointObservation) (runs.ModelCallCompleted, error) {
	consumption, err := accountModelEndpoint(callID, source)
	if err != nil {
		return runs.ModelCallCompleted{}, err
	}
	return o.session.accounting.accountModelCall(invocation, consumption)
}

func (o *observedInteractionModel) stopFailedCall(ctx context.Context, invocation interaction.ModelInvocation, cause, projectionErr error) error {
	if projectionErr == nil {
		if cause != nil {
			o.session.modelFailures.record(invocation.Relation().ProcessID(), cause)
		}
		kind := corechat.ClassifyFailure(cause)
		if kind == corechat.CallFailureFailed || kind == corechat.CallFailureNotExecuted {
			return cause
		}
		return errors.Join(cause, o.session.stopModelProcess(ctx, invocation.Relation().ProcessID()))
	}
	return errors.Join(cause, projectionErr)
}

// Settlement uses dispatch evidence even when no response survived. Scope owns
// the process outcome, including a consumer's capacity stop.
func (o *observedInteractionModel) settleFailedCall(ctx context.Context, invocation interaction.ModelInvocation, callID string, observation runs.ModelObservation, latency *int64) error {
	projectionCtx, cancel := o.session.lifetime.publicationContext(ctx)
	defer cancel()
	if err := o.session.flushDeltas(projectionCtx); err != nil {
		return err
	}
	source, err := o.endpoint(projectionCtx, callID)
	if err != nil {
		return err
	}
	failed := runs.ModelCallFailed{CallID: callID, Observation: observation, FirstOutputLatencyMillis: latency}
	if source != nil {
		accounted, err := o.accountEndpoint(invocation, callID, *source)
		if err != nil {
			return err
		}
		failed.Accounting = &accounted
		failed.KnownResponse = source.Response.Clone()
		if source.Response != nil && source.Response.Output.Message != nil {
			failed.Observation = runs.ModelObservation{}
			for _, part := range source.Response.Output.Message.Parts {
				switch part.Kind {
				case corechat.PartText, corechat.PartRefusal:
					failed.Observation.Text += part.Text
				case corechat.PartReasoning:
					failed.Observation.Reasoning += part.Text
				}
			}
		}
	}
	return o.session.commitFact(
		projectionCtx,
		o.session.executorMember(invocation.Relation()),
		failed,
	)
}

func (o *observedInteractionModel) begin(
	ctx context.Context,
) (
	invocation interaction.ModelInvocation,
	callID string,
	err error,
) {
	invocation, ok := interaction.ModelInvocationFromContext(ctx)
	if !ok {
		return interaction.ModelInvocation{}, "", errors.New("execution: model call has no Interaction attribution")
	}
	preparedInvocation := invocation
	defer func() {
		if err != nil {
			o.session.accounting.discardPreparedModelContext(preparedInvocation)
			o.session.modelFailures.record(preparedInvocation.Relation().ProcessID(), &run.FailureError{Kind: run.FailureInternal, Err: err})
			err = &corechat.CallFailure{Kind: corechat.CallFailureNotExecuted, Cause: err}
		}
	}()
	callIdentity, err := modelInvocationID(invocation)
	if err != nil {
		return interaction.ModelInvocation{}, "", err
	}
	callID = callIdentity.String()
	member := o.session.executorMember(invocation.Relation())
	o.session.retireToolReceiptPrefix(member.MemberID)
	if err := o.session.commitAppliedInputs(
		ctx, member, invocation.Relation().ProcessID(), invocation.AppliedSteerSignalIDs(),
	); err != nil {
		return interaction.ModelInvocation{}, "", err
	}
	if err := o.session.commitFact(ctx, member, runs.ModelCallStarted{CallID: callID}); err != nil {
		return interaction.ModelInvocation{}, "", fmt.Errorf("execution: commit model call start: %w", err)
	}
	return invocation, callID, nil
}

func (o *observedInteractionModel) complete(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
	firstOutputLatencyMillis *int64,
) error {
	modelOutput := response.Output
	// Agent owns Delta validation, ordering, buffering, and listener observation. Wait on its
	// ordering barrier before committing the authoritative full response so an
	// accepted stream increment can never reopen an Item after completion.
	projectionCtx, cancel := o.session.lifetime.publicationContext(ctx)
	defer cancel()
	if err := o.session.flushDeltas(projectionCtx); err != nil {
		return err
	}
	source, err := o.endpoint(projectionCtx, callID)
	if err != nil {
		return err
	}
	if source == nil || source.Response == nil {
		return errors.New("execution: completed model operation has no known physical endpoint evidence")
	}
	fact, err := o.accountEndpoint(invocation, callID, *source)
	if err != nil {
		return err
	}
	fact.FirstOutputLatencyMillis = firstOutputLatencyMillis
	fact.Message = nil
	if response.Output.Message != nil {
		fact.Message = new(response.Output.Message.Clone())
	}
	if err := o.session.commitFact(
		projectionCtx, o.session.executorMember(invocation.Relation()), fact,
	); err != nil {
		return err
	}
	if modelOutput.Message != nil {
		return o.session.registerDelegateCalls(invocation, modelOutput.Message)
	}
	return nil
}

// newObservedInteractionModel wraps the resolved provider so every model call
// is attributed before it leaves the Runtime. Streaming is offered only when the
// provider itself streams; the Dispatcher takes exactly one of the two.
func newObservedInteractionModel(
	model corechat.Model,
	streamer corechat.Streamer,
	session *interactionSession,
) (*observedInteractionModel, error) {
	if lo.IsNil(model) {
		return nil, errors.New("execution: Interaction model is required")
	}
	return &observedInteractionModel{model: model, streamer: streamer, session: session}, nil
}

// Streams reports whether this boundary can serve the Dispatcher's streaming
// capability.
func (o *observedInteractionModel) Streams() bool { return !lo.IsNil(o.streamer) }

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
