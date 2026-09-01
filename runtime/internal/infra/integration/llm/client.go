package llm

import (
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"

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

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

const (
	defaultAnthropicModel = "claude-opus-5"
	defaultOpenAIModel    = "gpt-5.6-sol"
)

type clientCredentialKind uint8

const (
	clientCredentialAbsent clientCredentialKind = iota + 1
	clientCredentialAPIKey
)

// ClientCredential explicitly represents an unauthenticated endpoint or one
// exact API key. Raw secret access remains inside the provider implementation.
type ClientCredential struct {
	kind   clientCredentialKind
	apiKey string
}

func NoClientCredential() ClientCredential {
	return ClientCredential{kind: clientCredentialAbsent}
}

func NewAPIKeyCredential(apiKey string) (ClientCredential, error) {
	if strings.TrimSpace(apiKey) == "" {
		return ClientCredential{}, fmt.Errorf("llm: API key is blank")
	}
	return ClientCredential{kind: clientCredentialAPIKey, apiKey: apiKey}, nil
}

func (c ClientCredential) validate() error {
	switch c.kind {
	case clientCredentialAbsent:
		if c.apiKey != "" {
			return fmt.Errorf("unauthenticated credential carries an API key")
		}
	case clientCredentialAPIKey:
		if strings.TrimSpace(c.apiKey) == "" {
			return fmt.Errorf("API key credential is blank")
		}
	default:
		return fmt.Errorf("unknown credential kind %d", c.kind)
	}
	return nil
}

func (c ClientCredential) sdkAPIKey() string {
	if c.kind != clientCredentialAPIKey {
		return ""
	}
	return c.apiKey
}

func (c ClientCredential) configured() bool { return c.kind == clientCredentialAPIKey }

type clientEndpointKind uint8

const (
	clientEndpointAbsent clientEndpointKind = iota + 1
	clientEndpointConfigured
)

type clientEndpoint struct {
	kind    clientEndpointKind
	baseURL string
}

func noClientEndpoint() clientEndpoint {
	return clientEndpoint{kind: clientEndpointAbsent}
}

func configuredClientEndpoint(baseURL string) (clientEndpoint, error) {
	if err := validateCatalogBaseURL(baseURL); err != nil {
		return clientEndpoint{}, err
	}
	return clientEndpoint{kind: clientEndpointConfigured, baseURL: baseURL}, nil
}

func (e clientEndpoint) validate() error {
	switch e.kind {
	case clientEndpointAbsent:
		if e.baseURL != "" {
			return fmt.Errorf("absent endpoint carries a base URL")
		}
	case clientEndpointConfigured:
		if err := validateCatalogBaseURL(e.baseURL); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown endpoint kind %d", e.kind)
	}
	return nil
}

func (e clientEndpoint) configured() bool { return e.kind == clientEndpointConfigured }

func (e clientEndpoint) sdkBaseURL() string {
	if !e.configured() {
		return ""
	}
	return e.baseURL
}

// ClientSpec is the constructed input for one chat or embedding client. Its
// fields are private so endpoint and credential absence cannot be encoded by
// blank strings or a partial struct literal.
type ClientSpec struct {
	provider   Provider
	model      modelref.ModelIdentity
	credential ClientCredential
	endpoint   clientEndpoint
}

func NewClientSpec(provider Provider, model string, credential ClientCredential) (ClientSpec, error) {
	identity, err := modelref.NewModelIdentity(model)
	if err != nil {
		return ClientSpec{}, fmt.Errorf("llm: model: %w", err)
	}
	spec := ClientSpec{provider: provider, model: identity, credential: credential, endpoint: noClientEndpoint()}
	if err := spec.validate(); err != nil {
		return ClientSpec{}, err
	}
	return spec, nil
}

func (s ClientSpec) WithBaseURL(baseURL string) (ClientSpec, error) {
	endpoint, err := configuredClientEndpoint(baseURL)
	if err != nil {
		return ClientSpec{}, fmt.Errorf("llm: base URL: %w", err)
	}
	s.endpoint = endpoint
	return s, nil
}

func (s ClientSpec) validate() error {
	profile, found := providers.lookup(s.provider)
	if !found {
		return fmt.Errorf("llm: unsupported provider %q", s.provider)
	}
	if _, err := modelref.NewModelIdentity(s.model.String()); err != nil {
		return fmt.Errorf("llm: model: %w", err)
	}
	if err := s.credential.validate(); err != nil {
		return fmt.Errorf("llm: credential: %w", err)
	}
	if profile.credential.required() && !s.credential.configured() {
		return fmt.Errorf("llm: provider %q requires an API key", s.provider)
	}
	if err := s.endpoint.validate(); err != nil {
		return fmt.Errorf("llm: endpoint: %w", err)
	}
	return nil
}

func (s ClientSpec) sdkAPIKey() string { return s.credential.sdkAPIKey() }

func (s ClientSpec) sdkBaseURL() string { return s.endpoint.sdkBaseURL() }

func (s ClientSpec) withEndpoint(endpoint clientEndpoint) ClientSpec {
	s.endpoint = endpoint
	return s
}

// buildFunc constructs the scope chat adapter for one (key, model, baseURL).
// One per provider — it is the only provider-specific chat construction code.
type buildFunc func(spec ClientSpec, opts chat.Options) (chat.Model, error)

// providers is the constructed provider catalog. Registration helpers encode
// the only legal combinations of endpoint ownership and model discovery;
// newProviderCatalog rejects duplicate identities and incomplete integrations.
var providers = mustProviderCatalog(
	// Direct vendor wire adapters (base URL optional — defaults to the vendor endpoint).
	bundledProvider(ProviderAnthropic, defaultAnthropicModel, "ANTHROPIC_API_KEY", buildAnthropicModel),
	bundledProvider(ProviderOpenAI, defaultOpenAIModel, "OPENAI_API_KEY", buildOpenAIResponsesModel).
		withEmbedding(bundledModels(defaultOpenAIEmbeddingModel), buildOpenAIEmbeddingModel),
	bundledProvider(ProviderGoogle, google.ModelGemini36Flash, "GOOGLE_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return google.NewChat(google.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}).withEmbedding(bundledModels(google.ModelGeminiEmbedding2), buildGoogleEmbeddingModel),

	// OpenAI-compatible vendors — each adapter encodes its own endpoint.
	bundledProvider(ProviderMoonshot, moonshot.ModelK3, "MOONSHOT_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return moonshot.NewChat(moonshot.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderDeepSeek, deepseek.ModelV4Flash, "DEEPSEEK_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return deepseek.NewChat(deepseek.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderAlibaba, alibaba.ModelQwen37Plus, "ALIBABA_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return alibaba.NewChat(alibaba.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}).withEmbedding(bundledModels(alibaba.ModelEmbeddingV4), buildAlibabaEmbeddingModel),
	bundledProvider(ProviderFireworks, fireworks.ModelGPTOSS20B, "FIREWORKS_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return fireworks.NewChat(fireworks.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderGroq, groq.ModelGPTOSS20B, "GROQ_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return groq.NewChat(groq.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderHuggingface, huggingface.ModelGPTOSS120B, "HUGGINGFACE_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return huggingface.NewChat(huggingface.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderMinimax, minimax.ModelM3, "MINIMAX_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return minimax.NewChat(minimax.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderMistral, mistral.ModelSmall, "MISTRAL_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return mistral.NewChat(mistral.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}).withEmbedding(bundledModels(mistral.ModelEmbed), buildMistralEmbeddingModel),
	bundledProvider(ProviderOpenRouter, openrouter.ModelAuto, "OPENROUTER_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return openrouter.NewChat(openrouter.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderPerplexity, perplexity.ModelSonar, "PERPLEXITY_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return perplexity.NewChat(perplexity.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderTogether, together.ModelRnj1Instruct, "TOGETHER_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return together.NewChat(together.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderXAI, xai.ModelGrok45, "XAI_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return xai.NewChat(xai.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderXiaomi, xiaomi.ModelV25Pro, "XIAOMI_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return xiaomi.NewChat(xiaomi.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}),
	bundledProvider(ProviderZhipu, zhipu.ModelGLM52, "ZHIPU_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return zhipu.NewChat(zhipu.ChatConfig{APIKey: s.sdkAPIKey(), DefaultOptions: o, BaseURL: s.sdkBaseURL()})
	}).withEmbedding(bundledModels(zhipu.ModelEmbedding3), buildZhipuEmbeddingModel),

	// Local daemon (base URL defaults to localhost; model id is user-pulled —
	// dynamic discovery probes the daemon's /v1/models for what is installed).
	optionalCredentialEndpointProvider(ProviderOllama, catalogEndpoint(defaultOllamaOpenAIBaseURL), "OLLAMA_API_KEY", buildOllamaChatModel).
		withEmbedding(bundledModels(defaultOllamaEmbeddingModel), buildOllamaEmbeddingModel),

	// Azure: the base URL is the complete per-resource /openai/v1 endpoint;
	// the model id is a deployment name. Both are user-supplied.
	endpointProvider(ProviderAzureOpenAI, configuredEndpoint(), "AZURE_OPENAI_API_KEY", func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return azureopenai.NewChat(azureopenai.ChatConfig{APIKey: s.sdkAPIKey(), BaseURL: s.sdkBaseURL(), DefaultOptions: o})
	}).withEmbedding(openAIEndpointModels(), buildAzureOpenAIEmbeddingModel),

	// Generic bring-your-own-endpoint providers: direct adapter + caller URL.
	endpointProvider(ProviderOpenAICompatible, configuredEndpoint(), "OPENAI_COMPATIBLE_API_KEY", buildOpenAICompatibleModel),
	endpointProvider(ProviderAnthropicCompatible, configuredEndpoint(), "ANTHROPIC_COMPATIBLE_API_KEY", buildAnthropicCompatibleModel).
		withChatModels(anthropicEndpointModels()),
)

func buildAnthropicCompatibleModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return buildAnthropicModel(spec, opts)
}

func buildAnthropicModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return anthropic.NewChat(anthropic.ChatConfig{
		APIKey:         spec.sdkAPIKey(),
		DefaultOptions: opts,
		BaseURL:        spec.sdkBaseURL(),
	})
}

func buildOpenAIResponsesModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewResponses(openai.ResponsesConfig{
		APIKey:         spec.sdkAPIKey(),
		DefaultOptions: opts,
		BaseURL:        spec.sdkBaseURL(),
	})
}

func buildOpenAICompatibleModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewChat(openai.ChatConfig{
		APIKey:         spec.sdkAPIKey(),
		DefaultOptions: opts,
		BaseURL:        spec.sdkBaseURL(),
	})
}

// buildModel selects and configures one provider model from the catalog. A
// provider that requires a base URL errors when one is not supplied. Pricing is
// a separate accounting concern, so the constructed model carries no pricing
// hook.
func buildModel(spec ClientSpec) (chat.Model, error) {
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

	model, err := profile.chatBuilder(spec, opts)
	if err != nil {
		return nil, fmt.Errorf("llm: build %s model: %w", spec.provider, err)
	}
	return classifyModelFailures(model), nil
}

// BuildChat constructs one provider model and projects both its ordinary chat
// client and its optional complete-request token counter. Capability discovery
// must happen on this exact model instance so callers never resolve credentials
// or build a provider twice for one interaction.
func BuildChat(spec ClientSpec) (*chatclient.Client, InputTokenCounter, error) {
	model, err := buildModel(spec)
	if err != nil {
		return nil, nil, err
	}

	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("llm: chat client: %w", err)
	}
	counter, _ := model.(InputTokenCounter)
	return &client, counter, nil
}
