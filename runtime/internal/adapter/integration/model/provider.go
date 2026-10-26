// Package model owns provider configuration, model construction, capability
// preservation and observed role resolution. Application owns the persisted
// credential registry; each resolution reads its current configuration. Scope
// owns the provider protocol primitives used by the resulting model instance.
package model

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
	providerAnthropic   Provider = "anthropic"
	providerOpenAI      Provider = "openai"
	providerMoonshot    Provider = "moonshot" // Kimi (OpenAI-compatible)
	providerDeepSeek    Provider = "deepseek" // DeepSeek (OpenAI-compatible)
	providerAlibaba     Provider = "alibaba"  // Qwen
	providerAzureOpenAI Provider = "azureopenai"
	providerFireworks   Provider = "fireworks"
	providerGoogle      Provider = "google" // Gemini
	providerGroq        Provider = "groq"
	providerHuggingface Provider = "huggingface"
	providerMinimax     Provider = "minimax"
	providerMistral     Provider = "mistral"
	providerOpenRouter  Provider = "openrouter"
	providerPerplexity  Provider = "perplexity"
	providerTogether    Provider = "together"
	providerXAI         Provider = "xai" // Grok
	providerXiaomi      Provider = "xiaomi"
	providerZhipu       Provider = "zhipu" // GLM

	// Generic "bring-your-own-endpoint" providers: the user supplies the base
	// URL + key + model id, and the Run executes through the OpenAI- / Anthropic-
	// wire adapter. They cover any compatible gateway not named above (and have
	// no catalog — the model id is user-supplied).
	providerOpenAICompatible    Provider = "openai-compatible"
	providerAnthropicCompatible Provider = "anthropic-compatible"
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

// supportedProviders lists every constructed profile in registration order,
// regardless of which providers are configured at runtime. Application use
// cases own any public catalog order.
func supportedProviders() []ProviderProfile {
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

func (p ProviderProfile) RequiresConfiguredEndpoint() bool {
	return p.value.endpoint.requiresConfiguration()
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

// EnvKeys reads the environment once and returns every provider API key that is
// present, keyed by provider id. Credential presence and provider readiness are
// deliberately separate: Azure and compatible providers still need a base URL,
// but retaining their key's environment provenance lets a later endpoint update
// make them usable without ever copying the secret into durable storage.
func EnvKeys() map[string]string {
	out := make(map[string]string)
	for _, provider := range providers.supported() {
		profile, _ := providers.lookup(provider)
		if key := os.Getenv(profile.credential.environment); key != "" {
			out[string(provider)] = key
		}
	}
	return out
}
