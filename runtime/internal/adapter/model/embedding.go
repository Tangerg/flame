package model

import (
	"context"
	"errors"

	"github.com/Tangerg/scope/core/embeddingclient"

	agentmemoryapp "github.com/Tangerg/flame/runtime/internal/application/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/llm"
)

// EmbeddingResolver builds embedding clients from the current provider-registry
// snapshot. Construction is intentionally uncached so credential generations
// do not accumulate for the process lifetime. Persisted vector identity remains
// provider/model/endpoint-owned and deliberately excludes credential rotation.
type EmbeddingResolver struct {
	providers CredentialLookup
}

// NewEmbeddingResolver returns a resolver over the provider credential lookup.
func NewEmbeddingResolver(providers CredentialLookup) *EmbeddingResolver {
	return &EmbeddingResolver{providers: providers}
}

// Resolve builds an embedder for the current selection and registry snapshot.
func (e *EmbeddingResolver) Resolve(ctx context.Context, selection modelref.Selection) (agentmemoryapp.Embedder, error) {
	if !selection.Configured() {
		return nil, errors.New("model: explicit model selection is required")
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
	spec, err := inputs.clientSpec(model)
	if err != nil {
		return nil, err
	}
	m, err := llm.BuildEmbeddingModel(spec)
	if err != nil {
		return nil, err
	}
	client, err := embeddingclient.New(m)
	if err != nil {
		return nil, err
	}
	created := &embedder{id: inputs.embeddingSpaceID(model), client: client}
	return created, nil
}

// ValidateEmbeddingModel implements the application role-validation port while
// keeping the usable embedder inside the adapter that owns it.
func (e *EmbeddingResolver) ValidateEmbeddingModel(ctx context.Context, providerID, model string) error {
	selection, err := modelref.New(providerID, model)
	if err != nil {
		return err
	}
	_, err = e.Resolve(ctx, selection)
	return err
}

// embedder adapts an embeddingclient.Client to the agent-memory search port.
type embedder struct {
	id     string
	client embeddingclient.Client
}

func (e *embedder) ID() string { return e.id }

func (e *embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vecs, err := e.client.EmbedTexts(ctx, texts)
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(vecs))
	for i, v := range vecs {
		f := make([]float32, len(v))
		for j, x := range v {
			f[j] = float32(x)
		}
		out[i] = f
	}
	return out, nil
}
