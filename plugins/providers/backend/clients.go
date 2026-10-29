package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/alibaba"
	"github.com/Tangerg/scope/models/anthropic"
	"github.com/Tangerg/scope/models/azureopenai"
	"github.com/Tangerg/scope/models/deepseek"
	"github.com/Tangerg/scope/models/fireworks"
	"github.com/Tangerg/scope/models/google"
	"github.com/Tangerg/scope/models/groq"
	"github.com/Tangerg/scope/models/huggingface"
	"github.com/Tangerg/scope/models/minimax"
	"github.com/Tangerg/scope/models/mistral"
	"github.com/Tangerg/scope/models/moonshot"
	"github.com/Tangerg/scope/models/openai"
	"github.com/Tangerg/scope/models/openrouter"
	"github.com/Tangerg/scope/models/perplexity"
	"github.com/Tangerg/scope/models/together"
	"github.com/Tangerg/scope/models/xai"
	"github.com/Tangerg/scope/models/xiaomi"
	"github.com/Tangerg/scope/models/zhipu"
)

const (
	defaultAnthropicModel = "claude-opus-5"
	defaultOpenAIModel    = "gpt-5.6-sol"
)

type clientSpec struct {
	connection modelprovider.Connection
	httpClient *http.Client
}

func (s clientSpec) Format(state fmt.State, _ rune) { s.connection.Format(state, 'v') }
func (s clientSpec) sdkAPIKey() string              { return s.connection.APIKey }
func (s clientSpec) sdkBaseURL() string             { return s.connection.BaseURL }
func (s clientSpec) sdkHTTPClient() *http.Client    { return s.httpClient }

func newClientSpec(connection modelprovider.Connection) (clientSpec, error) {
	if err := connection.Validate(); err != nil {
		return clientSpec{}, err
	}
	profile, ok := providers.lookup(Provider(connection.Provider))
	if !ok {
		return clientSpec{}, errors.New("model provider: unsupported provider")
	}
	if profile.profile.Authentication == modelprovider.KeyRequired && connection.APIKey == "" {
		return clientSpec{}, errors.New("model provider: API key is required")
	}
	if profile.profile.EndpointRequired && connection.BaseURL == "" {
		return clientSpec{}, errors.New("model provider: endpoint is required")
	}
	return clientSpec{connection: connection, httpClient: newModelHTTPClient()}, nil
}

type buildFunc func(context.Context, clientSpec, chat.Options) (chat.Model, error)

var providers = mustProviderCatalog(
	// Direct vendor wire adapters (base URL optional — defaults to the vendor endpoint).
	bundledProvider(providerAnthropic, defaultAnthropicModel, buildAnthropicModel),
	bundledProvider(providerOpenAI, defaultOpenAIModel, buildOpenAIResponsesModel).
		withEmbedding(defaultOpenAIEmbeddingModel, buildOpenAIEmbeddingModel),
	bundledProvider(providerGoogle, google.ModelGemini36Flash, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return google.NewChat(ctx, google.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(google.ModelGeminiEmbedding2, buildGoogleEmbeddingModel),

	// OpenAI-compatible vendors — each adapter encodes its own endpoint.
	bundledProvider(providerMoonshot, moonshot.ModelK3, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return moonshot.NewChatCompletions(ctx, moonshot.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerDeepSeek, deepseek.ModelFlash, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return deepseek.NewChatCompletions(ctx, deepseek.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerAlibaba, alibaba.ModelQwen37Plus, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return alibaba.NewChatCompletions(ctx, alibaba.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(alibaba.ModelEmbeddingV4, buildAlibabaEmbeddingModel),
	bundledProvider(providerFireworks, fireworks.ModelGPTOSS120B, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return fireworks.NewChatCompletions(ctx, fireworks.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerGroq, groq.ModelGPTOSS20B, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return groq.NewChatCompletions(ctx, groq.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerHuggingface, huggingface.ModelGPTOSS120B, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return huggingface.NewChatCompletions(ctx, huggingface.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerMinimax, minimax.ModelM3, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return minimax.NewChatCompletions(ctx, minimax.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerMistral, mistral.ModelSmall, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return mistral.NewChat(ctx, mistral.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(mistral.ModelEmbed, buildMistralEmbeddingModel),
	bundledProvider(providerOpenRouter, openrouter.ModelAuto, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return openrouter.NewChatCompletions(ctx, openrouter.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	endpointProvider(providerPerplexity, false, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return perplexity.NewResponses(ctx, perplexity.ResponsesConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerTogether, together.ModelGPTOSS120B, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return together.NewChatCompletions(ctx, together.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerXAI, xai.ModelGrok45, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return xai.NewChatCompletions(ctx, xai.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerXiaomi, xiaomi.ModelV25Pro, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return xiaomi.NewChatCompletions(ctx, xiaomi.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(providerZhipu, zhipu.ModelGLM52, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return zhipu.NewChatCompletions(ctx, zhipu.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(zhipu.ModelEmbedding3, buildZhipuEmbeddingModel),

	// Azure: the base URL is the complete per-resource /openai/v1 endpoint;
	// the model id is a deployment name. Both are user-supplied.
	endpointProvider(providerAzureOpenAI, true, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return azureopenai.NewChatCompletions(ctx, azureopenai.ChatCompletionsConfig{Config: azureopenai.Config{APIKey: s.sdkAPIKey(), BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()}, DefaultOptions: o})
	}).withEmbedding("", buildAzureOpenAIEmbeddingModel),

	// Generic bring-your-own-endpoint providers: direct adapter + caller URL.
	endpointProvider(providerOpenAICompatible, true, buildOpenAICompatibleModel),
	endpointProvider(providerAnthropicCompatible, true, buildAnthropicModel),
)

func buildAnthropicModel(ctx context.Context, spec clientSpec, opts chat.Options) (chat.Model, error) {
	return anthropic.NewMessages(ctx, anthropic.MessagesConfig{
		APIKey:         spec.sdkAPIKey(),
		DefaultOptions: opts,
		BaseURL:        spec.sdkBaseURL(),
		HTTPClient:     spec.sdkHTTPClient(),
	})
}

func buildOpenAIResponsesModel(ctx context.Context, spec clientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewResponses(ctx, openai.ResponsesConfig{
		APIKey:         spec.sdkAPIKey(),
		DefaultOptions: opts,
		BaseURL:        spec.sdkBaseURL(),
		HTTPClient:     spec.sdkHTTPClient(),
	})
}

func buildOpenAICompatibleModel(ctx context.Context, spec clientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewChatCompletions(ctx, openai.ChatCompletionsConfig{
		APIKey:         spec.sdkAPIKey(),
		DefaultOptions: opts,
		BaseURL:        spec.sdkBaseURL(),
		HTTPClient:     spec.sdkHTTPClient(),
	})
}

func buildModel(ctx context.Context, spec clientSpec, modelID string) (chat.Model, error) {
	identity, err := modelchoice.NewModelIdentity(modelID)
	if err != nil {
		return nil, err
	}
	profile, ok := providers.lookup(Provider(spec.connection.Provider))
	if !ok {
		return nil, errors.New("model provider: unsupported provider")
	}
	return profile.chatBuilder(ctx, spec, chat.Options{Model: identity.String()})
}
