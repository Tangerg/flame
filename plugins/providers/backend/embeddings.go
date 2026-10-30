package main

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

type embeddingBuildFunc func(context.Context, clientSpec, embedding.Options) (embedding.Model, error)

func buildEmbeddingModel(ctx context.Context, spec clientSpec, modelID string) (embedding.Model, error) {
	identity, err := modelchoice.NewModelIdentity(modelID)
	if err != nil {
		return nil, fmt.Errorf("model provider: embedding identity: %w", err)
	}
	profile, ok := providers.lookup(Provider(spec.connection.Provider))
	if !ok || profile.embeddingBuilder == nil {
		return nil, errors.New("model provider: embeddings are unavailable")
	}
	return profile.embeddingBuilder(ctx, spec, embedding.Options{Model: identity.String()})
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
