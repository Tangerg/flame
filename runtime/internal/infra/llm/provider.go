// Package llm owns Flame's static provider catalog and constructs a chat client
// for a selected provider. Each catalog entry binds a vendor, default model,
// credential environment key, and external wire adapter.
//
// The runtime-mutable credential registry (a provider's configured key + base
// URL) is a separate concern. This package answers what providers exist and how
// to construct their clients, not what credentials are configured now.
package llm

import (
	"os"
)

// Provider identifies an LLM vendor Flame supports. Its lowercase string value
// is the stable catalog key.
type Provider string

const (
	// Named vendors with a model catalog. Each routes through its own adapter,
	// which encodes the vendor endpoint. IAM-only vendors (amazonbedrock, vertexai) are
	// intentionally absent — they don't fit the "paste an API key" model.
	ProviderAnthropic   Provider = "anthropic"
	ProviderOpenAI      Provider = "openai"
	ProviderMoonshot    Provider = "moonshot" // Kimi (OpenAI-compatible)
	ProviderDeepSeek    Provider = "deepseek" // DeepSeek (OpenAI-compatible)
	ProviderAlibaba     Provider = "alibaba"  // Qwen
	ProviderAzureOpenAI Provider = "azureopenai"
	ProviderFireworks   Provider = "fireworks"
	ProviderGoogle      Provider = "google" // Gemini
	ProviderGroq        Provider = "groq"
	ProviderHuggingface Provider = "huggingface"
	ProviderMinimax     Provider = "minimax"
	ProviderMistral     Provider = "mistral"
	ProviderOllama      Provider = "ollama" // local
	ProviderOpenRouter  Provider = "openrouter"
	ProviderPerplexity  Provider = "perplexity"
	ProviderTogether    Provider = "together"
	ProviderXAI         Provider = "xai" // Grok
	ProviderXiaomi      Provider = "xiaomi"
	ProviderZhipu       Provider = "zhipu" // GLM

	// Generic "bring-your-own-endpoint" providers: the user supplies the base
	// URL + key + model id, and the Run executes through the OpenAI- / Anthropic-
	// wire adapter. They cover any compatible gateway not named above (and have
	// no catalog — the model id is user-supplied).
	ProviderOpenAICompatible    Provider = "openai-compatible"
	ProviderAnthropicCompatible Provider = "anthropic-compatible"
)

// ProviderProfile is the immutable constructed view of one provider
// integration. Its behavior derives endpoint, discovery, and embedding facts
// from closed policies instead of exposing the catalog's representation.
type ProviderProfile struct {
	value providerProfile
}

// LookupProvider resolves one exact provider identity. Absence is represented
// by the lookup result, never by an empty profile or empty metadata strings.
func LookupProvider(id Provider) (ProviderProfile, bool) {
	profile, found := providers.lookup(id)
	return ProviderProfile{value: profile}, found
}

// SupportedProviders lists every constructed profile in deterministic identity
// order, regardless of which providers are configured at runtime.
func SupportedProviders() []ProviderProfile {
	ids := providers.supported()
	out := make([]ProviderProfile, len(ids))
	for index, id := range ids {
		profile, _ := providers.lookup(id)
		out[index] = ProviderProfile{value: profile}
	}
	return out
}

func (p ProviderProfile) ID() Provider { return p.value.id }

// DefaultChatModel returns the bundled default. Endpoint-owned catalogs make
// absence explicit because their model identity is selected from live data.
func (p ProviderProfile) DefaultChatModel() (string, bool) {
	return p.value.chatModels.defaultValue()
}

func (p ProviderProfile) CredentialEnvironment() string { return p.value.credential.environment }

func (p ProviderProfile) RequiresAPIKey() bool { return p.value.credential.required() }

func (p ProviderProfile) RequiresConfiguredEndpoint() bool {
	return p.value.endpoint.requiresConfiguration()
}

// DefaultEndpoint returns the catalog-owned endpoint when one exists. Adapter-
// owned and caller-required endpoints are distinct policies, not empty values.
func (p ProviderProfile) DefaultEndpoint() (string, bool) {
	return p.value.endpoint.defaultValue()
}

func (p ProviderProfile) DiscoversModelsAtEndpoint() bool {
	return p.value.chatModels.discoveredAtEndpoint()
}

func (p ProviderProfile) SupportsEmbeddings() bool { return p.value.embedding != nil }

func (p ProviderProfile) DefaultEmbeddingModel() (string, bool) {
	if p.value.embedding == nil {
		return "", false
	}
	return p.value.embedding.models.defaultValue()
}

// EnvKeys reads the environment once and returns the API keys present for the
// providers a key alone makes usable — keyed by provider id, value the key. It
// backs the provider registry's stored>env credential fallback (a developer
// with ANTHROPIC_API_KEY / OPENAI_API_KEY / … in their shell gets those
// providers enabled out of the box).
//
// Providers that require a caller-supplied base URL (Azure and the compatible
// endpoint providers) are excluded: an env key alone can't reach their
// endpoint, so surfacing them as "enabled from env" would be a lie. The
// environment is process-static, so callers read this once at startup.
func EnvKeys() map[string]string {
	out := make(map[string]string)
	for _, provider := range providers.supported() {
		profile, _ := providers.lookup(provider)
		if !profile.endpoint.environmentCredentialIsUsable() {
			continue
		}
		if key := os.Getenv(profile.credential.environment); key != "" {
			out[string(provider)] = key
		}
	}
	return out
}
