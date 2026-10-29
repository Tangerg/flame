package main

import (
	"fmt"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/scope/models/catalog"
	"slices"
)

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

type providerProfile struct {
	profile          modelprovider.Profile
	chatBuilder      buildFunc
	embeddingBuilder embeddingBuildFunc
}

func bundledProvider(id Provider, defaultModel string, builder buildFunc) providerProfile {
	return providerProfile{profile: modelprovider.Profile{ID: string(id), Authentication: modelprovider.KeyRequired, ModelSource: modelprovider.BundledModels, DefaultModel: defaultModel, Models: catalog.Default.Models(string(id))}, chatBuilder: builder}
}
func endpointProvider(id Provider, endpointRequired bool, builder buildFunc) providerProfile {
	return providerProfile{profile: modelprovider.Profile{ID: string(id), Authentication: modelprovider.KeyRequired, EndpointRequired: endpointRequired, ModelSource: modelprovider.EndpointModels, Models: []catalog.Model{}}, chatBuilder: builder}
}
func (p providerProfile) withEmbedding(defaultModel string, builder embeddingBuildFunc) providerProfile {
	p.profile.Embedding = &modelprovider.EmbeddingProfile{DefaultModel: defaultModel}
	p.embeddingBuilder = builder
	return p
}

type providerCatalog struct{ profiles []providerProfile }

func mustProviderCatalog(profiles ...providerProfile) providerCatalog {
	seen := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		if err := p.profile.Validate(); err != nil {
			panic(fmt.Errorf("model provider: %s: %w", p.profile.ID, err))
		}
		if seen[p.profile.ID] || p.chatBuilder == nil || (p.profile.Embedding != nil && p.embeddingBuilder == nil) {
			panic("model provider: invalid profile construction")
		}
		seen[p.profile.ID] = true
	}
	return providerCatalog{profiles: profiles}
}
func (c providerCatalog) lookup(id Provider) (providerProfile, bool) {
	i := slices.IndexFunc(c.profiles, func(p providerProfile) bool { return p.profile.ID == string(id) })
	if i < 0 {
		return providerProfile{}, false
	}
	return c.profiles[i], true
}
func (c providerCatalog) published() []modelprovider.Profile {
	result := make([]modelprovider.Profile, 0, len(c.profiles))
	for _, p := range c.profiles {
		result = append(result, p.profile)
	}
	return result
}
