package llm

import (
	"fmt"

	"github.com/Tangerg/scope/core/embedding"

	"github.com/Tangerg/scope/models/alibaba"
	"github.com/Tangerg/scope/models/azureopenai"
	"github.com/Tangerg/scope/models/google"
	"github.com/Tangerg/scope/models/mistral"
	openaimodel "github.com/Tangerg/scope/models/openai"
	"github.com/Tangerg/scope/models/zhipu"
)

const (
	defaultOpenAIEmbeddingModel = "text-embedding-3-small"
	defaultOllamaEmbeddingModel = "nomic-embed-text"
)

// embeddingBuildFunc constructs an embedding adapter for one (key, model, baseURL).
type embeddingBuildFunc func(spec ClientSpec, opts embedding.Options) (embedding.Model, error)

type embeddingProviderProfile struct {
	models modelPolicy
	build  embeddingBuildFunc
}

func (p embeddingProviderProfile) validate() error {
	if err := p.models.validate(); err != nil {
		return err
	}
	if p.build == nil {
		return fmt.Errorf("builder is nil")
	}
	return nil
}

// BuildEmbeddingModel wires an embedding.Model for one provider+model from
// the provider profile, threading the credential and resolved endpoint.
func BuildEmbeddingModel(spec ClientSpec) (embedding.Model, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	provider, found := providers.lookup(spec.provider)
	if !found || provider.embedding == nil {
		return nil, fmt.Errorf("llm: provider %q has no embeddings adapter", spec.provider)
	}
	endpoint, err := provider.endpoint.resolve(spec.endpoint)
	if err != nil {
		return nil, fmt.Errorf("llm: provider %q: %w", spec.provider, err)
	}
	spec = spec.withEndpoint(endpoint)
	profile := provider.embedding
	opts := embedding.Options{Model: spec.model.String()}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("llm: embedding options for %q: %w", spec.model.String(), err)
	}
	model, err := profile.build(spec, opts)
	if err != nil {
		return nil, fmt.Errorf("llm: build %s embedding model: %w", spec.provider, err)
	}
	return model, nil
}

func buildOpenAIEmbeddingModel(s ClientSpec, o embedding.Options) (embedding.Model, error) {
	return openaimodel.NewEmbeddingModel(openaimodel.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}

func buildAzureOpenAIEmbeddingModel(s ClientSpec, o embedding.Options) (embedding.Model, error) {
	return azureopenai.NewEmbeddingModel(azureopenai.EmbeddingModelConfig{Config: azureopenai.Config{APIKey: s.sdkAPIKey(), BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()}, DefaultOptions: o})
}

func buildGoogleEmbeddingModel(s ClientSpec, o embedding.Options) (embedding.Model, error) {
	return google.NewEmbeddingModel(google.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}

func buildMistralEmbeddingModel(s ClientSpec, o embedding.Options) (embedding.Model, error) {
	return mistral.NewEmbeddingModel(mistral.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}

func buildZhipuEmbeddingModel(s ClientSpec, o embedding.Options) (embedding.Model, error) {
	return zhipu.NewEmbeddingModel(zhipu.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}

func buildAlibabaEmbeddingModel(s ClientSpec, o embedding.Options) (embedding.Model, error) {
	return alibaba.NewEmbeddingModel(alibaba.EmbeddingModelConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
}
