package main

import (
	"fmt"
	"slices"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/scope/models/catalog"
)

type Provider string

const (
	// IAM-only vendors (amazonbedrock, vertexai) are intentionally absent: they do not
	// fit the API-key model.
	providerAnthropic   Provider = "anthropic"
	providerOpenAI      Provider = "openai"
	providerMoonshot    Provider = "moonshot"
	providerDeepSeek    Provider = "deepseek"
	providerAlibaba     Provider = "alibaba"
	providerAzureOpenAI Provider = "azureopenai"
	providerFireworks   Provider = "fireworks"
	providerGoogle      Provider = "google"
	providerGroq        Provider = "groq"
	providerHuggingface Provider = "huggingface"
	providerMinimax     Provider = "minimax"
	providerMistral     Provider = "mistral"
	providerOpenRouter  Provider = "openrouter"
	providerPerplexity  Provider = "perplexity"
	providerTogether    Provider = "together"
	providerXAI         Provider = "xai"
	providerXiaomi      Provider = "xiaomi"
	providerZhipu       Provider = "zhipu"

	providerOpenAICompatible    Provider = "openai-compatible"
	providerAnthropicCompatible Provider = "anthropic-compatible"
)

type providerProfile struct {
	profile          modelprovider.Profile
	chatBuilder      buildFunc
	embeddingBuilder embeddingBuildFunc
}

func bundledProvider(id Provider, endpoint flamemodel.EndpointPolicy, defaultModel string, builder buildFunc) providerProfile {
	return providerProfile{profile: modelprovider.Profile{ID: string(id), Endpoint: endpoint, Authentication: modelprovider.KeyRequired, ModelSource: modelprovider.BundledModels, DefaultModel: defaultModel, Models: catalog.Models(string(id))}, chatBuilder: builder}
}

type endpointSupply uint8

const (
	providerEndpoint endpointSupply = iota
	userEndpoint
)

func endpointProvider(id Provider, endpoint flamemodel.EndpointPolicy, supply endpointSupply, builder buildFunc) providerProfile {
	return providerProfile{profile: modelprovider.Profile{ID: string(id), Endpoint: endpoint, Authentication: modelprovider.KeyRequired, EndpointRequired: supply == userEndpoint, ModelSource: modelprovider.EndpointModels, Models: []catalog.Model{}}, chatBuilder: builder}
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
