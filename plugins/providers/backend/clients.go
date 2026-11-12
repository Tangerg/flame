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
	anthropicprotocol "github.com/Tangerg/scope/models/protocol/anthropic"
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
	if err := profile.profile.Endpoint.ValidateOverride(connection.BaseURL); err != nil {
		return clientSpec{}, err
	}
	if profile.profile.EndpointRequired && connection.BaseURL == "" {
		return clientSpec{}, errors.New("model provider: endpoint is required")
	}
	return clientSpec{connection: connection, httpClient: newModelHTTPClient()}, nil
}

type buildFunc func(context.Context, clientSpec, chat.Options) (chat.Model, error)

const (
	// These are gateway examples, not SDK defaults. Version-owning protocols
	// append their API version after the supplied mount point.
	gatewayRoot  = "https://gateway.example/model-service"
	gatewayV1    = gatewayRoot + "/v1"
	azureAPIPath = "/openai/v1"
	azureGateway = "https://resource.openai.azure.com" + azureAPIPath
)

var (
	anthropicMount = modelchoice.EndpointPolicy{Example: gatewayRoot, AppendsPath: "/v1"}
	googleMount    = modelchoice.EndpointPolicy{Example: gatewayRoot, AppendsPath: "/v1beta"}
	apiMount       = modelchoice.EndpointPolicy{Example: gatewayV1}
	azureMount     = modelchoice.EndpointPolicy{Example: azureGateway, RequiredSuffix: azureAPIPath}
)

var providers = mustProviderCatalog(
	// Direct vendor wire adapters (base URL optional — defaults to the vendor endpoint).
	bundledProvider(providerAnthropic, anthropicMount, defaultAnthropicModel, buildAnthropicModel),
	bundledProvider(providerOpenAI, apiMount, defaultOpenAIModel, buildOpenAIResponsesModel).
		withEmbedding(defaultOpenAIEmbeddingModel, buildOpenAIEmbeddingModel),
	bundledProvider(providerGoogle, googleMount, google.ModelGemini36Flash, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return google.NewChat(ctx, google.ChatConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}).withEmbedding(google.ModelGeminiEmbedding2, buildGoogleEmbeddingModel),

	// OpenAI-compatible vendors — each adapter encodes its own endpoint.
	bundledProvider(providerMoonshot, apiMount, moonshot.ModelK3, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return moonshot.NewChatCompletions(ctx, moonshot.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerDeepSeek, apiMount, deepseek.ModelFlash, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return deepseek.NewChatCompletions(ctx, deepseek.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerAlibaba, apiMount, alibaba.ModelQwen37Plus, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return alibaba.NewChatCompletions(ctx, alibaba.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}).withEmbedding(alibaba.ModelEmbeddingV4, buildAlibabaEmbeddingModel),
	bundledProvider(providerFireworks, apiMount, fireworks.ModelGPTOSS120B, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return fireworks.NewChatCompletions(ctx, fireworks.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerGroq, apiMount, groq.ModelGPTOSS20B, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return groq.NewChatCompletions(ctx, groq.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerHuggingface, apiMount, huggingface.ModelGPTOSS120B, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return huggingface.NewChatCompletions(ctx, huggingface.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerMinimax, apiMount, minimax.ModelM3, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return minimax.NewChatCompletions(ctx, minimax.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerMistral, apiMount, mistral.ModelSmall, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return mistral.NewChat(ctx, mistral.ChatConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}).withEmbedding(mistral.ModelEmbed, buildMistralEmbeddingModel),
	bundledProvider(providerOpenRouter, apiMount, openrouter.ModelAuto, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return openrouter.NewChatCompletions(ctx, openrouter.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	endpointProvider(providerPerplexity, apiMount, false, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return perplexity.NewResponses(ctx, perplexity.ResponsesConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerTogether, apiMount, together.ModelGPTOSS120B, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return together.NewChatCompletions(ctx, together.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerXAI, apiMount, xai.ModelGrok45, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return xai.NewChatCompletions(ctx, xai.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerXiaomi, apiMount, xiaomi.ModelV25Pro, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return xiaomi.NewChatCompletions(ctx, xiaomi.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}),
	bundledProvider(providerZhipu, apiMount, zhipu.ModelGLM52, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return zhipu.NewChatCompletions(ctx, zhipu.ChatCompletionsConfig{APIKey: s.connection.APIKey, DefaultOptions: o, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient})
	}).withEmbedding(zhipu.ModelEmbedding3, buildZhipuEmbeddingModel),

	// Azure: the base URL is the complete per-resource /openai/v1 endpoint;
	// the model id is a deployment name. Both are user-supplied.
	endpointProvider(providerAzureOpenAI, azureMount, true, func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return azureopenai.NewChatCompletions(ctx, azureopenai.ChatCompletionsConfig{Config: azureopenai.Config{APIKey: s.connection.APIKey, BaseURL: s.connection.BaseURL, HTTPClient: s.httpClient}, DefaultOptions: o})
	}).withEmbedding("", buildAzureOpenAIEmbeddingModel),

	// Generic bring-your-own-endpoint providers: direct adapter + caller URL.
	endpointProvider(providerOpenAICompatible, apiMount, true, buildOpenAICompatibleModel),
	endpointProvider(providerAnthropicCompatible, anthropicMount, true, buildAnthropicModel),
)

func buildAnthropicModel(ctx context.Context, spec clientSpec, opts chat.Options) (chat.Model, error) {
	if Provider(spec.connection.Provider) == providerAnthropic {
		// Native Messages requires an explicit cache opt-in. A generic compatible
		// endpoint makes no promise to implement this vendor extension. Use Scope's
		// public request extension so request overrides retain their normal meaning.
		defaults := chat.Options{Model: opts.Model}
		if err := defaults.Extensions.Set(anthropicprotocol.RequestExtensionKey, map[string]any{
			"cache_control": map[string]any{"type": "ephemeral"},
		}); err != nil {
			return nil, err
		}
		var err error
		opts, err = defaults.Resolve(opts)
		if err != nil {
			return nil, err
		}
	}
	return anthropic.NewMessages(ctx, anthropic.MessagesConfig{
		APIKey:         spec.connection.APIKey,
		DefaultOptions: opts,
		BaseURL:        spec.connection.BaseURL,
		HTTPClient:     spec.httpClient,
	})
}

func buildOpenAIResponsesModel(ctx context.Context, spec clientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewResponses(ctx, openai.ResponsesConfig{
		APIKey:         spec.connection.APIKey,
		DefaultOptions: opts,
		BaseURL:        spec.connection.BaseURL,
		HTTPClient:     spec.httpClient,
	})
}

func buildOpenAICompatibleModel(ctx context.Context, spec clientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewChatCompletions(ctx, openai.ChatCompletionsConfig{
		APIKey:         spec.connection.APIKey,
		DefaultOptions: opts,
		BaseURL:        spec.connection.BaseURL,
		HTTPClient:     spec.httpClient,
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
