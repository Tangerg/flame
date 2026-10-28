package model

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
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
}

// EmbeddingResolver builds observed embedding clients from the current
// provider-registry snapshot. Construction is intentionally uncached so
// credential generations do not accumulate for the process lifetime. Persisted
// vector identity remains provider/model/endpoint-owned and deliberately
// excludes credential rotation. Every resolved embedder journals its real
// provider request through the shared endpoint evidence owner.
type EmbeddingResolver struct {
	providers credentialLookup
	journal   endpointJournal
	pricing   accounting.Pricing
}

// NewEmbeddingResolver returns a complete resolver over the provider credential
// lookup. The endpoint journal owns physical admission and settlement; pricing
// is optional and absent when no price table is configured.
func NewEmbeddingResolver(providers credentialLookup, journal endpointJournal, pricing accounting.Pricing) (*EmbeddingResolver, error) {
	if lo.IsNil(providers) {
		return nil, errors.New("model: embedding provider credential lookup is required")
	}
	if lo.IsNil(journal) {
		return nil, errors.New("model: embedding endpoint journal is required")
	}
	return &EmbeddingResolver{providers: providers, journal: journal, pricing: pricing}, nil
}

// Resolve builds an embedder for the current selection and registry snapshot.
func (e *EmbeddingResolver) Resolve(ctx context.Context, selection modelchoice.Selection) (TextEmbedder, error) {
	if e == nil || lo.IsNil(e.providers) || lo.IsNil(e.journal) {
		return nil, errors.New("model: embedding resolver is not configured")
	}
	if err := selection.ValidateExact(); err != nil {
		return nil, fmt.Errorf("model: embedding selection: %w", err)
	}
	providerID, model := selection.Provider(), selection.Model()
	entry, ok, err := e.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		entry, err = provider.New(providerID)
		if err != nil {
			return nil, err
		}
	}
	inputs, err := resolveProviderClientInputs(providerID, entry)
	if err != nil {
		return nil, err
	}
	spec, err := inputs.clientSpec()
	if err != nil {
		return nil, err
	}
	m, err := buildEmbeddingModel(ctx, spec, model)
	if err != nil {
		return nil, err
	}
	created := &embedder{id: inputs.embeddingSpaceID(model), model: m, selection: selection, journal: e.journal, pricing: e.pricing}
	return created, nil
}

// ValidateEmbeddingModel implements the application role-validation port while
// keeping the usable embedder inside the adapter that owns it.
func (e *EmbeddingResolver) ValidateEmbeddingModel(ctx context.Context, providerID, model string) error {
	selection, err := modelchoice.New(providerID, model)
	if err != nil {
		return err
	}
	_, err = e.Resolve(ctx, selection)
	return err
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
}

func (e *embedder) ID() string { return e.id }

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
	response, cause := e.model.Call(ctx, cloneEmbeddingRequest(request))
	if response != nil && cause == nil {
		cause = response.ValidateFor(request)
	}
	if response == nil && cause == nil {
		cause = errors.New("model: embedding returned no response")
	}
	settleErr := e.finish(ctx, invocation, response, cause)
	if cause = errors.Join(cause, settleErr); cause != nil {
		return nil, cause
	}
	out := make([][]float32, len(response.Outputs))
	for index, output := range response.Outputs {
		out[index] = embedding.Float32Vector(output.Embedding)
	}
	return out, nil
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
	if response != nil && response.Validate() != nil {
		cause = errors.Join(cause, response.Validate())
		response = nil
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
