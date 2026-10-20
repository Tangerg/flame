package model

import (
	"context"
	"errors"
	"iter"
	"time"

	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// EndpointJournal commits observations without authorizing replay or continuation.
// The owner is the host-captured logical Model/Tool call, not caller-supplied ancestry.
// Its durable admission supplies the original Run/Segment before provider generation.
type EndpointJournal interface {
	StartEndpointInvocation(context.Context, modelref.EndpointInvocation, string) error
	SettleEndpointInvocation(context.Context, modelref.EndpointInvocation, modelref.EndpointOutcome) error
}

type ObservedResolver struct {
	resolver ChatModelResolver
	journal  EndpointJournal
	pricing  accounting.Pricing
}

func NewObservedResolver(resolver ChatModelResolver, journal EndpointJournal, pricing accounting.Pricing) (*ObservedResolver, error) {
	if dependency.Missing(resolver) || dependency.Missing(journal) {
		return nil, errors.New("model: resolver and endpoint journal are required")
	}
	return &ObservedResolver{resolver: resolver, journal: journal, pricing: pricing}, nil
}

func (r *ObservedResolver) ResolveChat(ctx context.Context, selection modelchoice.Selection) (ResolvedChat, error) {
	resolved, err := r.resolver.ResolveChat(ctx, selection)
	if err != nil {
		return ResolvedChat{}, err
	}
	if dependency.Missing(resolved.Model()) {
		return ResolvedChat{}, errors.New("model: resolver returned no endpoint model")
	}
	model := &observedEndpoint{inner: resolved.Model(), journal: r.journal, selection: selection, pricing: r.pricing}
	var bound chat.Model = model
	if stream, ok := resolved.Streamer(); ok {
		bound = &streamedEndpoint{observedEndpoint: model, stream: stream}
	}
	counter, _ := resolved.InputTokenCounter()
	return NewResolvedChat(bound, counter)
}

func (r *ObservedResolver) ValidateChatModel(ctx context.Context, provider, model string) error {
	selection, err := modelchoice.New(provider, model)
	if err != nil {
		return err
	}
	_, err = r.ResolveChat(ctx, selection)
	return err
}

type observedEndpoint struct {
	inner     chat.Model
	journal   EndpointJournal
	selection modelchoice.Selection
	pricing   accounting.Pricing
}

func (m *observedEndpoint) start(ctx context.Context, request *chat.Request) (modelref.EndpointInvocation, error) {
	if err := request.Validate(); err != nil {
		return modelref.EndpointInvocation{}, err
	}
	if request.Options.Model != "" && request.Options.Model != m.selection.Model() {
		return modelref.EndpointInvocation{}, errors.New("model: request cannot replace the captured endpoint selection")
	}
	caller, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil {
		return modelref.EndpointInvocation{}, err
	}
	invocation := modelref.EndpointInvocation{ID: "endpoint_" + uuid.NewString(), Kind: modelref.EndpointChat, Caller: caller, Provider: m.selection.Provider(), Model: m.selection.Model(), SessionID: executionctx.SessionID(ctx), Request: request.Clone(), StartedAt: time.Now().UTC()}
	if err := invocation.Validate(); err != nil {
		return modelref.EndpointInvocation{}, err
	}
	if err := m.journal.StartEndpointInvocation(ctx, invocation, executionctx.InvocationOwner(ctx)); err != nil {
		return modelref.EndpointInvocation{}, err
	}
	return invocation, nil
}

func (m *observedEndpoint) finish(ctx context.Context, invocation modelref.EndpointInvocation, response *chat.Response, cause error) error {
	if response != nil && response.Validate() != nil {
		cause = errors.Join(cause, response.Validate())
		response = nil
	}
	failure := ""
	if cause != nil {
		failure = cause.Error()
	}
	var cost accounting.Cost
	if response != nil && response.Metadata != nil && response.Metadata.Usage != nil && m.pricing != nil {
		served := response.Metadata.Model
		if served == "" {
			served = invocation.Model
		}
		cost = m.pricing(invocation.Provider, served, response.Metadata.Usage)
	}
	return m.journal.SettleEndpointInvocation(context.WithoutCancel(ctx), invocation, modelref.EndpointOutcome{CostUSD: cost.OptionalUSD(), Response: response.Clone(), FinishedAt: time.Now().UTC(), Failure: failure})
}

func (m *observedEndpoint) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	invocation, err := m.start(ctx, request)
	if err != nil {
		return nil, err
	}
	response, err := m.inner.Call(ctx, invocation.Request.Clone())
	if response != nil {
		err = errors.Join(err, response.Validate())
	}
	if response == nil && err == nil {
		err = errors.New("model: endpoint returned no response")
	}
	return response, errors.Join(err, m.finish(ctx, invocation, response, err))
}

type streamedEndpoint struct {
	*observedEndpoint
	stream chat.Streamer
}

func (m *streamedEndpoint) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	frozen := request.Clone()
	return func(yield func(*chat.ResponseDelta, error) bool) {
		invocation, err := m.start(ctx, frozen)
		if err != nil {
			yield(nil, err)
			return
		}
		var accumulated chat.ResponseAccumulator
		settled := false
		defer func() {
			if !settled {
				// Stop returns no successful observation. Startup reconciles a
				// failed publication as unknown without replaying this request.
				response, _ := accumulated.Response()
				if err := m.finish(ctx, invocation, response, errors.Join(ctx.Err(), context.Cause(ctx), errors.New("model: endpoint stream consumption stopped"))); err != nil {
					trace.SpanFromContext(ctx).RecordError(err)
				}
			}
		}()
		sequence := m.stream.Stream(ctx, invocation.Request.Clone())
		if sequence == nil {
			err = errors.New("model: endpoint returned no stream")
		} else {
			for delta, cause := range sequence {
				if cause != nil {
					err = cause
					break
				}
				if cause := accumulated.Add(delta); cause != nil {
					err = cause
					break
				}
				if !yield(delta.Clone(), nil) {
					return
				}
			}
		}
		response, responseErr := accumulated.Response()
		if responseErr != nil {
			response = nil
			err = errors.Join(err, responseErr)
		}
		publicationErr := m.finish(ctx, invocation, response, err)
		settled = true
		if err = errors.Join(err, publicationErr); err != nil {
			yield(nil, err)
		}
	}
}
