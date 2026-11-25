package main

import (
	"context"
	"errors"
	"fmt"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
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

func (s clientSpec) openEmbedding(ctx context.Context, modelID string) (embedding.Model, error) {
	identity, err := flamemodel.NewModelIdentity(modelID)
	if err != nil {
		return nil, fmt.Errorf("model provider: embedding identity: %w", err)
	}
	if s.profile.embeddingBuilder == nil {
		return nil, errors.New("model provider: embeddings are unavailable")
	}
	return s.profile.embeddingBuilder(ctx, s, embedding.Options{Model: identity.String()})
}

func buildOpenAIEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return openaimodel.NewEmbeddingModel(ctx, openaimodel.EmbeddingModelConfig{APIKey: s.apiKey, DefaultOptions: o, BaseURL: s.baseURL, HTTPClient: s.httpClient})
}

func buildAzureOpenAIEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return azureopenai.NewEmbeddingModel(ctx, azureopenai.EmbeddingModelConfig{Config: azureopenai.Config{APIKey: s.apiKey, BaseURL: s.baseURL, HTTPClient: s.httpClient}, DefaultOptions: o})
}

func buildGoogleEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return google.NewEmbeddingModel(ctx, google.EmbeddingModelConfig{APIKey: s.apiKey, DefaultOptions: o, BaseURL: s.baseURL, HTTPClient: s.httpClient})
}

func buildMistralEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return mistral.NewEmbeddingModel(ctx, mistral.EmbeddingModelConfig{APIKey: s.apiKey, DefaultOptions: o, BaseURL: s.baseURL, HTTPClient: s.httpClient})
}

func buildZhipuEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return zhipu.NewEmbeddingModel(ctx, zhipu.EmbeddingModelConfig{APIKey: s.apiKey, DefaultOptions: o, BaseURL: s.baseURL, HTTPClient: s.httpClient})
}

func buildAlibabaEmbeddingModel(ctx context.Context, s clientSpec, o embedding.Options) (embedding.Model, error) {
	return alibaba.NewEmbeddingModel(ctx, alibaba.EmbeddingModelConfig{APIKey: s.apiKey, DefaultOptions: o, BaseURL: s.baseURL, HTTPClient: s.httpClient})
}
