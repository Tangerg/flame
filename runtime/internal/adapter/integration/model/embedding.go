package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

// TextEmbedder exposes the model adapter's captured vector space and observed call.
// A feature consumes this port without owning provider resolution or evidence.
type TextEmbedder interface {
	ID() string
	Embed(context.Context, []string) ([][]float32, error)
	Close(context.Context) error
}

// EmbeddingResolver builds observed embedding clients from the current
// provider-registry snapshot. Construction is intentionally uncached so
// credential generations do not accumulate for the process lifetime. Persisted
// vector identity remains provider/model/endpoint-owned and deliberately
// excludes credential rotation. Every resolved embedder journals its real
// provider request through the shared endpoint evidence owner.
type EmbeddingResolver struct {
	providers credentialLookup
	clients   embeddingFactory
	journal   endpointJournal
	pricing   accounting.Pricing
}

type embeddingFactory interface {
	OpenEmbedding(context.Context, modelchoice.Selection, modelprovider.Connection) (ResolvedEmbedding, error)
}

// NewEmbeddingResolver returns a complete resolver over the provider credential
// lookup. The endpoint journal owns physical admission and settlement; pricing
// is optional and absent when no price table is configured.
func NewEmbeddingResolver(providers credentialLookup, clients embeddingFactory, journal endpointJournal, pricing accounting.Pricing) (*EmbeddingResolver, error) {
	if lo.IsNil(providers) || lo.IsNil(clients) {
		return nil, errors.New("model: embedding provider credentials and installed adapter are required")
	}
	if lo.IsNil(journal) {
		return nil, errors.New("model: embedding endpoint journal is required")
	}
	return &EmbeddingResolver{providers: providers, clients: clients, journal: journal, pricing: pricing}, nil
}

// Resolve builds an embedder for the current selection and registry snapshot.
func (e *EmbeddingResolver) Resolve(ctx context.Context, selection modelchoice.Selection) (TextEmbedder, error) {
	if e == nil || lo.IsNil(e.providers) || lo.IsNil(e.journal) {
		return nil, errors.New("model: embedding resolver is not configured")
	}
	if err := selection.ValidateExact(); err != nil {
		return nil, fmt.Errorf("model: embedding selection: %w", err)
	}
	connection, err := resolveConnection(ctx, e.providers, selection)
	if err != nil {
		return nil, err
	}
	resolved, err := e.clients.OpenEmbedding(ctx, selection, connection)
	if err != nil {
		return nil, err
	}
	pricing := resolved.pricing
	if e.pricing != nil {
		pricing = e.pricing
	}
	created := &embedder{id: resolved.space, model: resolved.model, selection: selection, journal: e.journal, pricing: pricing, close: resolved.close}
	return created, nil
}

// ValidateEmbeddingModel implements the application role-validation port while
// keeping the usable embedder inside the adapter that owns it.
func (e *EmbeddingResolver) ValidateEmbeddingModel(ctx context.Context, providerID, model string) error {
	selection, err := modelchoice.New(providerID, model)
	if err != nil {
		return err
	}
	resolved, err := e.Resolve(ctx, selection)
	if err != nil {
		return err
	}
	return resolved.Close(ctx)
}

// embedder observes one embedding provider boundary: it admits the real request
// before dispatch, settles its known response or uncertainty through the shared
// endpoint journal, and projects the output vectors back to the search port. It
// never reconstructs a provider response from vectors.
type embedder struct {
	id        string
	model     embedding.Model
	selection modelchoice.Selection
	journal   endpointJournal
	pricing   accounting.Pricing
	close     func(context.Context) error
}

func (e *embedder) ID() string { return e.id }

func (e *embedder) Close(ctx context.Context) error {
	if e.close != nil {
		return e.close(ctx)
	}
	return nil
}

func (e *embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	request, err := embedding.NewRequest(texts)
	if err != nil {
		return nil, err
	}
	request.Options.Model = e.selection.Model()
	if err := request.Validate(); err != nil {
		return nil, err
	}
	invocation, err := e.start(ctx, request)
	if err != nil {
		return nil, err
	}
	calling, cause := endpointContext(ctx, invocation)
	var response *embedding.Response
	if cause == nil {
		response, cause = e.model.Call(calling, cloneEmbeddingRequest(request))
	} else {
		cause = &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: cause}
	}
	if response != nil && cause == nil {
		cause = response.ValidateFor(request)
	}
	if response == nil && cause == nil {
		cause = errors.New("model: embedding returned no response")
	}
	var vectors [][]float32
	if cause == nil {
		vectors, cause = projectEmbeddingVectors(response)
	}
	settleErr := e.finish(ctx, invocation, response, cause)
	if cause = errors.Join(cause, settleErr); cause != nil {
		return nil, cause
	}
	return vectors, nil
}

func projectEmbeddingVectors(response *embedding.Response) ([][]float32, error) {
	vectors := make([][]float32, len(response.Outputs))
	for index, output := range response.Outputs {
		vector := embedding.Float32Vector(output.Embedding)
		for coordinate, value := range vector {
			if math.IsInf(float64(value), 0) {
				return nil, fmt.Errorf("model: embedding output %d coordinate %d exceeds float32 range", index, coordinate)
			}
		}
		vectors[index] = vector
	}
	return vectors, nil
}

func (e *embedder) start(ctx context.Context, request *embedding.Request) (modelref.EndpointInvocation, error) {
	caller, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		return modelref.EndpointInvocation{}, err
	}
	invocation := modelref.EndpointInvocation{ID: "endpoint_" + uuid.NewString(), Kind: modelref.EndpointEmbedding, Caller: caller, Provider: e.selection.Provider(), Model: e.selection.Model(), SessionID: executionctx.SessionID(ctx), Embedding: cloneEmbeddingRequest(request), StartedAt: time.Now().UTC()}
	if err := invocation.Validate(); err != nil {
		return modelref.EndpointInvocation{}, err
	}
	if err := e.journal.StartEndpointInvocation(ctx, invocation, executionctx.InvocationOwner(ctx)); err != nil {
		return modelref.EndpointInvocation{}, err
	}
	return invocation, nil
}

func (e *embedder) finish(ctx context.Context, invocation modelref.EndpointInvocation, response *embedding.Response, cause error) error {
	if response != nil {
		if err := response.Validate(); err != nil {
			cause = errors.Join(cause, err)
			response = nil
		}
	}
	var cost accounting.Cost
	var pricingErr error
	if response != nil && response.Metadata != nil && response.Metadata.Usage != nil && e.pricing != nil {
		served := response.Metadata.Model
		if served == "" {
			served = invocation.Model
		}
		cost, pricingErr = e.pricing(invocation.Provider, served, &chat.Usage{InputTokens: response.Metadata.Usage.InputTokens})
	}
	failure := ""
	if err := errors.Join(cause, pricingErr); err != nil {
		failure = err.Error()
	}
	return errors.Join(pricingErr, e.journal.SettleEndpointInvocation(context.WithoutCancel(ctx), invocation, modelref.EndpointOutcome{CostUSD: cost.OptionalUSD(), EmbeddingResponse: response, FinishedAt: time.Now().UTC(), Failure: failure}))
}

func cloneEmbeddingRequest(r *embedding.Request) *embedding.Request {
	return &embedding.Request{Texts: slices.Clone(r.Texts), Options: r.Options.Clone()}
}
