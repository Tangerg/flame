package models

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func TestProviderMetadataRejectsIncompletePolicies(t *testing.T) {
	embeddingIdentity, err := modelref.NewModelIdentity("embedding")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewProviderMetadata(
		"provider", 0, ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability(),
	); err == nil {
		t.Fatal("NewProviderMetadata accepted a missing authentication policy")
	}
	cases := []struct {
		name      string
		id        string
		endpoint  ProviderEndpointPolicy
		models    ProviderModelSource
		embedding EmbeddingCapability
	}{
		{name: "blank identity", endpoint: ProviderEndpointOptional, models: ProviderModelsBundled, embedding: NoEmbeddingCapability()},
		{name: "oversized identity", id: strings.Repeat("p", modelref.MaximumProviderIdentityCharacters+1), endpoint: ProviderEndpointOptional, models: ProviderModelsBundled, embedding: NoEmbeddingCapability()},
		{name: "missing endpoint policy", id: "provider", models: ProviderModelsBundled, embedding: NoEmbeddingCapability()},
		{name: "missing model source", id: "provider", endpoint: ProviderEndpointOptional, embedding: NoEmbeddingCapability()},
		{name: "zero embedding capability", id: "provider", endpoint: ProviderEndpointOptional, models: ProviderModelsBundled},
		{name: "default on unsupported embedding", id: "provider", endpoint: ProviderEndpointOptional, models: ProviderModelsBundled, embedding: EmbeddingCapability{kind: embeddingUnsupported, defaultModel: embeddingIdentity}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewProviderMetadata(test.id, ProviderAPIKeyRequired, test.endpoint, test.models, test.embedding); err == nil {
				t.Fatal("NewProviderMetadata accepted incomplete provider policy")
			}
		})
	}
}

func TestModelOwnsValidatedIdentityAndCapabilitySnapshot(t *testing.T) {
	levels := []string{"low", "high"}
	details := &Details{Reasoning: true, ReasoningLevels: levels, Pricing: &Pricing{InputPerMillion: 1}}
	model, err := NewModel("openai", "gpt-5", details)
	if err != nil {
		t.Fatal(err)
	}
	levels[0] = "mutated"
	details.Pricing.InputPerMillion = 99
	first := model.Details()
	if model.Provider() != "openai" || model.ID() != "gpt-5" || first.ReasoningLevels[0] != "low" || first.Pricing.InputPerMillion != 1 {
		t.Fatalf("model leaked constructor state: identity=%q/%q details=%+v", model.Provider(), model.ID(), first)
	}
	first.ReasoningLevels[0] = "mutated again"
	if model.Details().ReasoningLevels[0] != "low" {
		t.Fatal("Model.Details leaked its owned capability snapshot")
	}
	if _, err := NewModel("openai", "bad model", nil); err == nil {
		t.Fatal("NewModel accepted a non-canonical identity")
	}
}

func TestEmbeddingCapabilityMakesDefaultPresenceExplicit(t *testing.T) {
	withoutDefault := EmbeddingCapabilityWithoutDefault()
	if !withoutDefault.Supported() {
		t.Fatal("embedding adapter should be supported")
	}
	if _, present := withoutDefault.DefaultModel(); present {
		t.Fatal("endpoint-owned embedding model reported a default")
	}

	withDefault, err := EmbeddingCapabilityWithDefault("embed-v1")
	if err != nil {
		t.Fatal(err)
	}
	if model, present := withDefault.DefaultModel(); !present || model != "embed-v1" {
		t.Fatalf("default = %q, %v", model, present)
	}
	if _, err := EmbeddingCapabilityWithDefault("   "); err == nil {
		t.Fatal("blank default embedding model was accepted")
	}
}
