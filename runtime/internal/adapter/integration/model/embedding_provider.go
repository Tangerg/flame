package model

import (
	"context"
	"errors"
	"fmt"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/alibaba"
	"github.com/Tangerg/scope/models/azureopenai"
	"github.com/Tangerg/scope/models/google"
	"github.com/Tangerg/scope/models/mistral"
	openaimodel "github.com/Tangerg/scope/models/openai"
	"github.com/Tangerg/scope/models/zhipu"
)

const defaultOpenAIEmbeddingModel = "text-embedding-3-small"

// embeddingBuildFunc constructs an embedding adapter for one (key, model, baseURL).
type embeddingBuildFunc func(ctx context.Context, spec clientSpec, opts embedding.Options) (embedding.Model, error)

type embeddingProviderProfile struct {
	models modelPolicy
	build  embeddingBuildFunc
}

func (p embeddingProviderProfile) validate() error {
	if err := p.models.validate(); err != nil {
		return err
	}
	if p.build == nil {
		return errors.New("builder is nil")
	}
	return nil
}

// buildEmbeddingModel wires an embedding.Model for one provider+model from
// the provider profile, threading the credential and resolved endpoint.
func buildEmbeddingModel(ctx context.Context, spec clientSpec, modelID string) (embedding.Model, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	identity, err := modelchoice.NewModelIdentity(modelID)
	if err != nil {
		return nil, fmt.Errorf("model: embedding model: %w", err)
	}
	provider, found := providers.lookup(spec.provider)
	if !found || provider.embedding == nil {
		return nil, fmt.Errorf("model: provider %q has no embeddings adapter", spec.provider)
	}
	endpoint, err := provider.endpoint.resolve(spec.endpoint)
	if err != nil {
		return nil, fmt.Errorf("model: provider %q: %w", spec.provider, err)
	}
	spec = spec.withEndpoint(endpoint)
	profile := provider.embedding
	opts := embedding.Options{Model: identity.String()}
	model, err := profile.build(ctx, spec, opts)
	if err != nil {
		return nil, fmt.Errorf("model: build %s embedding model: %w", spec.provider, err)
	}
	return model, nil
}

func buildOpenAIEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return openaimodel.NewEmbeddingModel(ctx, openaimodel.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}

func buildAzureOpenAIEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return azureopenai.NewEmbeddingModel(ctx, azureopenai.EmbeddingModelConfig{Config: azureopenai.Config{APIKey: s.sdkAPIKey(), BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()}, DefaultOptions: o})
}

func buildGoogleEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return google.NewEmbeddingModel(ctx, google.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}

func buildMistralEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return mistral.NewEmbeddingModel(ctx, mistral.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}

func buildZhipuEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return zhipu.NewEmbeddingModel(ctx, zhipu.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}

func buildAlibabaEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return alibaba.NewEmbeddingModel(ctx, alibaba.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}
