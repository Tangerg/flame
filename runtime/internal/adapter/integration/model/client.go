package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	providerdomain "github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
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

// clientSpec owns connection policy; model selection belongs to each operation.
type clientSpec struct {
	provider   Provider
	credential providerdomain.APIKey
	endpoint   providerdomain.BaseURL
	httpClient *http.Client
}

// fmt does not invoke a nested value's Formatter through an unexported field.
func (s clientSpec) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "{Provider:%s Credential:%v Endpoint:%s}",
		s.provider, s.credential, s.endpoint.String())
}

func newClientSpec(provider Provider, credential providerdomain.APIKey) (clientSpec, error) {
	spec := clientSpec{
		provider: provider, credential: credential,
		httpClient: newModelHTTPClient(),
	}
	if err := spec.validate(); err != nil {
		return clientSpec{}, err
	}
	return spec, nil
}

func (s clientSpec) WithBaseURL(baseURL string) (clientSpec, error) {
	endpoint, err := providerdomain.NewBaseURL(baseURL)
	if err != nil {
		return clientSpec{}, fmt.Errorf("model: base URL: %w", err)
	}
	s.endpoint = endpoint
	return s, nil
}

func (s clientSpec) validate() error {
	_, found := providers.lookup(s.provider)
	if !found {
		return fmt.Errorf("model: unsupported provider %q", s.provider)
	}
	if !s.credential.Present() {
		return fmt.Errorf("model: provider %q credential: %w", s.provider, providerdomain.ErrAPIKeyRequired)
	}
	if s.httpClient == nil || s.httpClient.Transport == nil {
		return errors.New("model: bounded HTTP client is required")
	}
	return nil
}

func (s clientSpec) sdkAPIKey() string { return s.credential.Reveal() }

func (s clientSpec) sdkBaseURL() string { return s.endpoint.String() }

func (s clientSpec) sdkHTTPClient() *http.Client { return s.httpClient }

func (s clientSpec) withEndpoint(endpoint providerdomain.BaseURL) clientSpec {
	s.endpoint = endpoint
	return s
}

// buildFunc constructs the Scope chat adapter with its connection and options.
// One per provider — it is the only provider-specific chat construction code.
type buildFunc func(ctx context.Context, spec clientSpec, opts chat.Options) (chat.Model, error)

// providers is the constructed provider catalog. Registration helpers encode
// the only legal combinations of endpoint ownership and model discovery;
// newProviderCatalog rejects duplicate identities and incomplete integrations.
var providers = mustProviderCatalog(
	// Direct vendor wire adapters (base URL optional — defaults to the vendor endpoint).
	bundledProvider(ProviderAnthropic, defaultAnthropicModel, "ANTHROPIC_API_KEY", buildAnthropicModel),
	bundledProvider(ProviderOpenAI, defaultOpenAIModel, "OPENAI_API_KEY", buildOpenAIResponsesModel).
		withEmbedding(bundledModels(defaultOpenAIEmbeddingModel), buildOpenAIEmbeddingModel),
	bundledProvider(ProviderGoogle, google.ModelGemini36Flash, "GOOGLE_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return google.NewChat(ctx, google.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(bundledModels(google.ModelGeminiEmbedding2), buildGoogleEmbeddingModel),

	// OpenAI-compatible vendors — each adapter encodes its own endpoint.
	bundledProvider(ProviderMoonshot, moonshot.ModelK3, "MOONSHOT_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return moonshot.NewChatCompletions(ctx, moonshot.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderDeepSeek, deepseek.ModelFlash, "DEEPSEEK_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return deepseek.NewChatCompletions(ctx, deepseek.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderAlibaba, alibaba.ModelQwen37Plus, "ALIBABA_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return alibaba.NewChatCompletions(ctx, alibaba.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(bundledModels(alibaba.ModelEmbeddingV4), buildAlibabaEmbeddingModel),
	bundledProvider(ProviderFireworks, fireworks.ModelGPTOSS120B, "FIREWORKS_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return fireworks.NewChatCompletions(ctx, fireworks.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderGroq, groq.ModelGPTOSS20B, "GROQ_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return groq.NewChatCompletions(ctx, groq.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderHuggingface, huggingface.ModelGPTOSS120B, "HUGGINGFACE_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return huggingface.NewChatCompletions(ctx, huggingface.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderMinimax, minimax.ModelM3, "MINIMAX_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return minimax.NewChatCompletions(ctx, minimax.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderMistral, mistral.ModelSmall, "MISTRAL_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return mistral.NewChat(ctx, mistral.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(bundledModels(mistral.ModelEmbed), buildMistralEmbeddingModel),
	bundledProvider(ProviderOpenRouter, openrouter.ModelAuto, "OPENROUTER_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return openrouter.NewChatCompletions(ctx, openrouter.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	endpointProvider(ProviderPerplexity, adapterEndpoint(), "PERPLEXITY_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return perplexity.NewResponses(ctx, perplexity.ResponsesConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderTogether, together.ModelGPTOSS120B, "TOGETHER_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return together.NewChatCompletions(ctx, together.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderXAI, xai.ModelGrok45, "XAI_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return xai.NewChatCompletions(ctx, xai.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderXiaomi, xiaomi.ModelV25Pro, "XIAOMI_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return xiaomi.NewChatCompletions(ctx, xiaomi.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderZhipu, zhipu.ModelGLM52, "ZHIPU_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return zhipu.NewChatCompletions(ctx, zhipu.ChatCompletionsConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(bundledModels(zhipu.ModelEmbedding3), buildZhipuEmbeddingModel),

	// Azure: the base URL is the complete per-resource /openai/v1 endpoint;
	// the model id is a deployment name. Both are user-supplied.
	endpointProvider(ProviderAzureOpenAI, configuredEndpoint(), "AZURE_OPENAI_API_KEY", func(ctx context.Context, s clientSpec, o chat.Options) (chat.Model, error) {
		return azureopenai.NewChatCompletions(ctx, azureopenai.ChatCompletionsConfig{Config: azureopenai.Config{APIKey: s.sdkAPIKey(), BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()}, DefaultOptions: o})
	}).withEmbedding(endpointModels(), buildAzureOpenAIEmbeddingModel),

	// Generic bring-your-own-endpoint providers: direct adapter + caller URL.
	endpointProvider(ProviderOpenAICompatible, configuredEndpoint(), "OPENAI_COMPATIBLE_API_KEY", buildOpenAICompatibleModel),
	endpointProvider(ProviderAnthropicCompatible, configuredEndpoint(), "ANTHROPIC_COMPATIBLE_API_KEY", buildAnthropicModel),
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

// buildModel selects and configures one provider model from the catalog. A
// provider that requires a base URL errors when one is not supplied. Pricing is
// a separate accounting concern, so the constructed model carries no pricing
// hook.
func buildModel(ctx context.Context, spec clientSpec, modelID string) (chat.Model, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	identity, err := modelchoice.NewModelIdentity(modelID)
	if err != nil {
		return nil, fmt.Errorf("model: model: %w", err)
	}
	profile, ok := providers.lookup(spec.provider)
	if !ok {
		return nil, fmt.Errorf("model: unsupported provider %q", spec.provider)
	}
	endpoint, err := profile.endpoint.resolve(spec.endpoint)
	if err != nil {
		return nil, fmt.Errorf("model: provider %q: %w", spec.provider, err)
	}
	spec = spec.withEndpoint(endpoint)

	opts := chat.Options{Model: identity.String()}

	model, err := profile.chatBuilder(ctx, spec, opts)
	if err != nil {
		return nil, fmt.Errorf("model: build %s model: %w", spec.provider, err)
	}
	instrumented, err := instrumentModel(spec.provider, model)
	if err != nil {
		return nil, err
	}
	return classifyModelFailures(instrumented)
}

// buildChat constructs one provider model and projects both its ordinary chat
// client and its optional complete-request token counter. Capability discovery
// must happen on this exact model instance so callers never resolve credentials
// or build a provider twice for one interaction.
func buildChat(ctx context.Context, spec clientSpec, modelID string) (chat.Model, InputTokenCounter, error) {
	model, err := buildModel(ctx, spec, modelID)
	if err != nil {
		return nil, nil, err
	}
	counter, _ := model.(InputTokenCounter)
	return model, counter, nil
}
