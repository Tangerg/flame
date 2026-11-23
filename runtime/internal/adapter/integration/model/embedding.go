package model

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	coreinvocation "github.com/Tangerg/scope/core/invocation"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	corevector "github.com/Tangerg/scope/core/vector"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

// Construction is uncached so credential generations do not accumulate for the
// process lifetime.
type EmbeddingResolver struct {
	providers credentialLookup
	clients   embeddingFactory
	journal   endpointJournal
}

type embeddingFactory interface {
	OpenEmbedding(context.Context, flamemodel.Selection, modelprovider.Connection) (ResolvedEmbedding, error)
}

func NewEmbeddingResolver(providers credentialLookup, clients embeddingFactory, journal endpointJournal) (*EmbeddingResolver, error) {
	if lo.IsNil(providers) || lo.IsNil(clients) {
		return nil, errors.New("model: embedding provider credentials and installed adapter are required")
	}
	if lo.IsNil(journal) {
		return nil, errors.New("model: embedding endpoint journal is required")
	}
	return &EmbeddingResolver{providers: providers, clients: clients, journal: journal}, nil
}

func (e *EmbeddingResolver) Resolve(ctx context.Context, selection flamemodel.Selection) (*Embedder, error) {
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
	created := &Embedder{id: resolved.space, model: resolved.model, selection: selection, journal: e.journal, pricing: resolved.pricing, close: resolved.close}
	return created, nil
}

func (e *EmbeddingResolver) ValidateEmbeddingModel(ctx context.Context, providerID, model string) error {
	return validateByOpening(ctx, providerID, model, e.Resolve)
}

type Embedder struct {
	id        string
	model     embedding.Model
	selection flamemodel.Selection
	journal   endpointJournal
	pricing   accounting.Pricing
	close     func(context.Context) error
}

func (e *Embedder) ID() string { return e.id }

func (e *Embedder) Close(ctx context.Context) error { return e.close(ctx) }

func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
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
		cause = &coreinvocation.Failure{Kind: coreinvocation.FailureNotExecuted, Cause: cause}
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
		vector := corevector.Coordinates[float32](corevector.Float32(output.Embedding))
		if err := vector.Validate(); err != nil {
			return nil, fmt.Errorf("model: embedding output %d exceeds float32 range: %w", index, err)
		}
		vectors[index] = vector
	}
	return vectors, nil
}

func (e *Embedder) start(ctx context.Context, request *embedding.Request) (modelref.EndpointInvocation, error) {
	caller, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return modelref.EndpointInvocation{}, err
	}
	invocation := modelref.EndpointInvocation{ID: modelref.NewEndpointID(uuid.NewString()), Caller: caller, Provider: e.selection.Provider(), Model: e.selection.Model(), SessionID: executionctx.SessionID(ctx), Request: modelref.EmbeddingEndpointRequest{Embedding: cloneEmbeddingRequest(request)}, StartedAt: time.Now().UTC()}
	if err := invocation.Validate(); err != nil {
		return modelref.EndpointInvocation{}, err
	}
	if err := e.journal.StartEndpointInvocation(ctx, invocation, executionctx.InvocationOwner(ctx)); err != nil {
		return modelref.EndpointInvocation{}, err
	}
	return invocation, nil
}

func (e *Embedder) finish(ctx context.Context, invocation modelref.EndpointInvocation, response *embedding.Response, cause error) error {
	if response != nil {
		if err := response.Validate(); err != nil {
			cause = errors.Join(cause, err)
			response = nil
		}
	}
	var cost accounting.Cost
	var pricingErr error
	if response != nil && response.Metadata != nil && response.Metadata.Usage != nil {
		served := response.Metadata.Model
		if served == "" {
			served = invocation.Model
		}
		cost, pricingErr = e.pricing(invocation.Provider, served, &chat.Usage{InputTokens: response.Metadata.Usage.InputTokens})
	}
	outcome := modelref.EndpointOutcome{Cost: cost}
	if response != nil {
		outcome.Response = modelref.EmbeddingEndpointResponse{Embedding: response}
	}
	return recordEndpointOutcome(ctx, e.journal, invocation, outcome, cause, pricingErr)
}

func cloneEmbeddingRequest(r *embedding.Request) *embedding.Request {
	return &embedding.Request{Texts: slices.Clone(r.Texts), Options: r.Options.Clone()}
}
