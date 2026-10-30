package model

import (
	"context"
	"errors"
	"iter"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/trace"
)

// EndpointJournal commits observations without authorizing replay or continuation.
// The owner is the host-captured logical Model/Tool call, not caller-supplied ancestry.
// Its durable admission supplies the original Run/Segment before provider generation.
type endpointJournal interface {
	StartEndpointInvocation(context.Context, modelref.EndpointInvocation, string) error
	SettleEndpointInvocation(context.Context, modelref.EndpointInvocation, modelref.EndpointOutcome) error
}

type ObservedResolver struct {
	resolver ChatModelResolver
	journal  endpointJournal
	pricing  accounting.Pricing
}

func NewObservedResolver(resolver ChatModelResolver, journal endpointJournal, pricing accounting.Pricing) (*ObservedResolver, error) {
	if lo.IsNil(resolver) || lo.IsNil(journal) {
		return nil, errors.New("model: resolver and endpoint journal are required")
	}
	return &ObservedResolver{resolver: resolver, journal: journal, pricing: pricing}, nil
}

func (r *ObservedResolver) ResolveChat(ctx context.Context, selection modelchoice.Selection) (ResolvedChat, error) {
	resolved, err := r.resolver.ResolveChat(ctx, selection)
	if err != nil {
		return ResolvedChat{}, err
	}
	return r.observe(selection, resolved)
}

func (r *ObservedResolver) RestoreChat(ctx context.Context, selection modelchoice.Selection, capture *ProviderCapture) (ResolvedChat, error) {
	resolved, err := r.resolver.RestoreChat(ctx, selection, capture)
	if err != nil {
		return ResolvedChat{}, err
	}
	return r.observe(selection, resolved)
}

func (r *ObservedResolver) observe(selection modelchoice.Selection, resolved ResolvedChat) (ResolvedChat, error) {
	if lo.IsNil(resolved.Model()) {
		return ResolvedChat{}, errors.New("model: resolver returned no endpoint model")
	}
	pricing := resolved.pricing
	if r.pricing != nil {
		pricing = r.pricing
	}
	model := &observedEndpoint{inner: resolved.Model(), journal: r.journal, selection: selection, pricing: pricing}
	var bound chat.Model = model
	if stream, ok := resolved.Streamer(); ok {
		bound = &streamedEndpoint{observedEndpoint: model, stream: stream}
	}
	return resolved.WithModel(bound)
}

func (r *ObservedResolver) ValidateChatModel(ctx context.Context, provider, model string) error {
	selection, err := modelchoice.New(provider, model)
	if err != nil {
		return err
	}
	resolved, err := r.ResolveChat(ctx, selection)
	if err != nil {
		return err
	}
	return resolved.Close(ctx)
}

type observedEndpoint struct {
	inner     chat.Model
	journal   endpointJournal
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
	caller, err := qimumcp.InvocationFromContext(ctx)
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
	var cost accounting.Cost
	var pricingErr error
	if response != nil && response.Metadata != nil && response.Metadata.Usage != nil && m.pricing != nil {
		served := response.Metadata.Model
		if served == "" {
			served = invocation.Model
		}
		cost, pricingErr = m.pricing(invocation.Provider, served, response.Metadata.Usage)
	}
	failure := ""
	if err := errors.Join(cause, pricingErr); err != nil {
		failure = err.Error()
	}
	return errors.Join(pricingErr, m.journal.SettleEndpointInvocation(context.WithoutCancel(ctx), invocation, modelref.EndpointOutcome{CostUSD: cost.OptionalUSD(), Response: response.Clone(), FinishedAt: time.Now().UTC(), Failure: failure}))
}

func (m *observedEndpoint) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	invocation, err := m.start(ctx, request)
	if err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	calling, err := endpointContext(ctx, invocation)
	var response *chat.Response
	if err == nil {
		response, err = m.inner.Call(calling, invocation.Request.Clone())
	} else {
		err = &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	var admitted *chat.Response
	if response != nil {
		validationErr := response.Validate()
		err = errors.Join(err, validationErr)
		if validationErr == nil {
			admitted = response
		}
	}
	if response == nil && err == nil {
		err = errors.New("model: endpoint returned no response")
	}
	return response, errors.Join(err, m.finish(ctx, invocation, admitted, err))
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
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
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
		calling, err := endpointContext(ctx, invocation)
		if err != nil {
			yield(nil, errors.Join(&chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}, m.finish(ctx, invocation, nil, err)))
			settled = true
			return
		}
		sequence := m.stream.Stream(calling, invocation.Request.Clone())
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
			if err == nil {
				err = responseErr
			}
		}
		publicationErr := m.finish(ctx, invocation, response, err)
		settled = true
		if err = errors.Join(err, publicationErr); err != nil {
			yield(nil, err)
		}
	}
}

func endpointContext(ctx context.Context, invocation modelref.EndpointInvocation) (context.Context, error) {
	return qimumcp.WithInvocation(ctx, portable.Invocation{ID: invocation.ID, Origin: invocation.Caller.Origin, Parent: invocation.Caller.ID, Target: portable.Target{Scope: extension.ScopeCall, Reference: invocation.ID}})
}
