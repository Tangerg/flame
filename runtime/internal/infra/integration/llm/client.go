package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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

	providerdomain "github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

const (
	defaultAnthropicModel = "claude-opus-5"
	defaultOpenAIModel    = "gpt-5.6-sol"
)

type ClientSpec struct {
	provider   Provider
	model      modelref.ModelIdentity
	credential providerdomain.APIKey
	endpoint   providerdomain.BaseURL
	httpClient *http.Client
}

// fmt does not invoke a nested value's Formatter through an unexported field.
func (s ClientSpec) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "{Provider:%s Model:%s Credential:%v Endpoint:%s}",
		s.provider, s.model.String(), s.credential, s.endpoint.String())
}

func NewClientSpec(provider Provider, model string, credential providerdomain.APIKey) (ClientSpec, error) {
	identity, err := modelref.NewModelIdentity(model)
	if err != nil {
		return ClientSpec{}, fmt.Errorf("llm: model: %w", err)
	}
	spec := ClientSpec{
		provider: provider, model: identity, credential: credential,
		httpClient: newModelHTTPClient(),
	}
	if err := spec.validate(); err != nil {
		return ClientSpec{}, err
	}
	return spec, nil
}

func (s ClientSpec) WithBaseURL(baseURL string) (ClientSpec, error) {
	endpoint, err := providerdomain.NewBaseURL(baseURL)
	if err != nil {
		return ClientSpec{}, fmt.Errorf("llm: base URL: %w", err)
	}
	s.endpoint = endpoint
	return s, nil
}

func (s ClientSpec) validate() error {
	_, found := providers.lookup(s.provider)
	if !found {
		return fmt.Errorf("llm: unsupported provider %q", s.provider)
	}
	if s.model.String() == "" {
		return errors.New("llm: model identity is required")
	}
	if !s.credential.Present() {
		return fmt.Errorf("llm: provider %q credential: %w", s.provider, providerdomain.ErrAPIKeyRequired)
	}
	if s.httpClient == nil || s.httpClient.Transport == nil {
		return errors.New("llm: bounded HTTP client is required")
	}
	return nil
}

func (s ClientSpec) sdkAPIKey() string { return s.credential.Reveal() }

func (s ClientSpec) sdkBaseURL() string { return s.endpoint.String() }

func (s ClientSpec) sdkHTTPClient() *http.Client { return s.httpClient }

func (s ClientSpec) withEndpoint(endpoint providerdomain.BaseURL) ClientSpec {
	s.endpoint = endpoint
	return s
}

// buildFunc constructs the scope chat adapter for one (key, model, baseURL).
// One per provider — it is the only provider-specific chat construction code.
type buildFunc func(ctx context.Context, spec ClientSpec, opts chat.Options) (chat.Model, error)

// providers is the constructed provider catalog. Registration helpers encode
// the only legal combinations of endpoint ownership and model discovery;
// newProviderCatalog rejects duplicate identities and incomplete integrations.
var providers = mustProviderCatalog(
	// Direct vendor wire adapters (base URL optional — defaults to the vendor endpoint).
	bundledProvider(ProviderAnthropic, defaultAnthropicModel, "ANTHROPIC_API_KEY", buildAnthropicModel),
	bundledProvider(ProviderOpenAI, defaultOpenAIModel, "OPENAI_API_KEY", buildOpenAIResponsesModel).
		withEmbedding(bundledModels(defaultOpenAIEmbeddingModel), buildOpenAIEmbeddingModel),
	bundledProvider(ProviderGoogle, google.ModelGemini36Flash, "GOOGLE_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return google.NewChat(ctx, google.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(bundledModels(google.ModelGeminiEmbedding2), buildGoogleEmbeddingModel),

	// OpenAI-compatible vendors — each adapter encodes its own endpoint.
	bundledProvider(ProviderMoonshot, moonshot.ModelK3, "MOONSHOT_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return moonshot.NewChat(ctx, moonshot.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderDeepSeek, deepseek.ModelFlash, "DEEPSEEK_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return deepseek.NewChat(ctx, deepseek.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderAlibaba, alibaba.ModelQwen37Plus, "ALIBABA_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return alibaba.NewChat(ctx, alibaba.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(bundledModels(alibaba.ModelEmbeddingV4), buildAlibabaEmbeddingModel),
	bundledProvider(ProviderFireworks, fireworks.ModelGPTOSS120B, "FIREWORKS_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return fireworks.NewChat(ctx, fireworks.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderGroq, groq.ModelGPTOSS20B, "GROQ_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return groq.NewChat(ctx, groq.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderHuggingface, huggingface.ModelGPTOSS120B, "HUGGINGFACE_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return huggingface.NewChat(ctx, huggingface.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderMinimax, minimax.ModelM3, "MINIMAX_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return minimax.NewChat(ctx, minimax.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderMistral, mistral.ModelSmall, "MISTRAL_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return mistral.NewChat(ctx, mistral.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(bundledModels(mistral.ModelEmbed), buildMistralEmbeddingModel),
	bundledProvider(ProviderOpenRouter, openrouter.ModelAuto, "OPENROUTER_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return openrouter.NewChat(ctx, openrouter.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderPerplexity, perplexity.ModelSonar, "PERPLEXITY_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return perplexity.NewChat(ctx, perplexity.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderTogether, together.ModelGPTOSS120B, "TOGETHER_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return together.NewChat(ctx, together.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderXAI, xai.ModelGrok45, "XAI_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return xai.NewChat(ctx, xai.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderXiaomi, xiaomi.ModelV25Pro, "XIAOMI_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return xiaomi.NewChat(ctx, xiaomi.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}),
	bundledProvider(ProviderZhipu, zhipu.ModelGLM52, "ZHIPU_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return zhipu.NewChat(ctx, zhipu.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()})
	}).withEmbedding(bundledModels(zhipu.ModelEmbedding3), buildZhipuEmbeddingModel),

	// Azure: the base URL is the complete per-resource /openai/v1 endpoint;
	// the model id is a deployment name. Both are user-supplied.
	endpointProvider(ProviderAzureOpenAI, configuredEndpoint(), "AZURE_OPENAI_API_KEY", func(ctx context.Context, s ClientSpec, o chat.Options) (chat.Model, error) {
		return azureopenai.NewChat(ctx, azureopenai.ChatConfig{Config: azureopenai.Config{APIKey: s.sdkAPIKey(), BaseURL: s.sdkBaseURL(), HTTPClient: s.sdkHTTPClient()}, DefaultOptions: o})
	}).withEmbedding(openAIEndpointModels(), buildAzureOpenAIEmbeddingModel),

	// Generic bring-your-own-endpoint providers: direct adapter + caller URL.
	endpointProvider(ProviderOpenAICompatible, configuredEndpoint(), "OPENAI_COMPATIBLE_API_KEY", buildOpenAICompatibleModel),
	endpointProvider(ProviderAnthropicCompatible, configuredEndpoint(), "ANTHROPIC_COMPATIBLE_API_KEY", buildAnthropicCompatibleModel).
		withChatModels(anthropicEndpointModels()),
)

func buildAnthropicCompatibleModel(ctx context.Context, spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return buildAnthropicModel(ctx, spec, opts)
}

func buildAnthropicModel(ctx context.Context, spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return anthropic.NewChat(ctx, anthropic.ChatConfig{
		APIKey:         spec.sdkAPIKey(),
		DefaultOptions: opts,
		BaseURL:        spec.sdkBaseURL(),
		HTTPClient:     spec.sdkHTTPClient(),
	})
}

func buildOpenAIResponsesModel(ctx context.Context, spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewResponses(ctx, openai.ResponsesConfig{
		APIKey:         spec.sdkAPIKey(),
		DefaultOptions: opts,
		BaseURL:        spec.sdkBaseURL(),
		HTTPClient:     spec.sdkHTTPClient(),
	})
}

func buildOpenAICompatibleModel(ctx context.Context, spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewChat(ctx, openai.ChatConfig{
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
func buildModel(ctx context.Context, spec ClientSpec) (chat.Model, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	profile, ok := providers.lookup(spec.provider)
	if !ok {
		return nil, fmt.Errorf("llm: unsupported provider %q", spec.provider)
	}
	endpoint, err := profile.endpoint.resolve(spec.endpoint)
	if err != nil {
		return nil, fmt.Errorf("llm: provider %q: %w", spec.provider, err)
	}
	spec = spec.withEndpoint(endpoint)

	opts := chat.Options{Model: spec.model.String()}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("llm: chat options for %q: %w", spec.model.String(), err)
	}

	model, err := profile.chatBuilder(ctx, spec, opts)
	if err != nil {
		return nil, fmt.Errorf("llm: build %s model: %w", spec.provider, err)
	}
	instrumented, err := instrumentModel(spec.provider, model)
	if err != nil {
		return nil, err
	}
	return classifyModelFailures(instrumented)
}

// BuildChat constructs one provider model and projects both its ordinary chat
// client and its optional complete-request token counter. Capability discovery
// must happen on this exact model instance so callers never resolve credentials
// or build a provider twice for one interaction.
func BuildChat(ctx context.Context, spec ClientSpec) (chat.Model, InputTokenCounter, error) {
	model, err := buildModel(ctx, spec)
	if err != nil {
		return nil, nil, err
	}
	counter, _ := model.(InputTokenCounter)
	return model, counter, nil
}
