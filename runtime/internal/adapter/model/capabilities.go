// Package model adapts provider configuration, static catalogs, chat clients,
// and embedding clients to Runtime application ports.
package model

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/scope/core/chat"
	catalog "github.com/Tangerg/scope/models/catalog"

	modelsapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/llm"
)

// Capabilities implements the three model-configuration ports consumed by
// model use cases: static catalog lookup, credential probing, and remote listing.
// Keeping them together is justified because they share one provider
// integration boundary.
type Capabilities struct{}

const (
	providerProbePrompt      = "ping"
	minimumProbeOutputTokens = int64(1)
	configurationProbeModel  = "flame-configuration-probe"
)

func (Capabilities) Supported() []modelsapp.ProviderMetadata {
	supported := llm.SupportedProviders()
	out := make([]modelsapp.ProviderMetadata, 0, len(supported))
	for _, value := range supported {
		out = append(out, providerMetadata(value))
	}
	return out
}

func (Capabilities) Metadata(id string) (modelsapp.ProviderMetadata, bool) {
	profile, found := llm.LookupProvider(llm.Provider(id))
	if !found {
		return modelsapp.ProviderMetadata{}, false
	}
	return providerMetadata(profile), true
}

func (Capabilities) Models(providerID string) []modelsapp.Model {
	entries := catalog.Default.Models(providerID)
	out := make([]modelsapp.Model, 0, len(entries))
	for _, entry := range entries {
		out = append(out, catalogModel(providerID, entry))
	}
	return out
}

func (Capabilities) LookupModel(providerID, modelID string) (modelsapp.Model, bool) {
	entry, ok := catalog.Default.Lookup(providerID, modelID)
	if !ok {
		return modelsapp.Model{}, false
	}
	return catalogModel(providerID, entry), true
}

func (Capabilities) Probe(ctx context.Context, entry provider.Provider) error {
	inputs, err := resolveProviderClientInputs(entry.ID(), entry)
	if err != nil {
		return err
	}
	if inputs.profile.DiscoversModelsAtEndpoint() {
		models, err := remoteModelIDs(ctx, inputs)
		if err != nil {
			return err
		}
		if len(models) == 0 {
			return fmt.Errorf("model: provider %q advertised no models", entry.ID())
		}
		return nil
	}
	defaultModel, hasDefault := inputs.profile.DefaultChatModel()
	if !hasDefault {
		return fmt.Errorf("model: provider %q has no probe model", entry.ID())
	}
	spec, err := inputs.clientSpec(defaultModel)
	if err != nil {
		return err
	}
	client, _, err := llm.BuildChat(spec)
	if err != nil {
		return err
	}
	maxTokens := minimumProbeOutputTokens
	_, err = client.Call(ctx, &chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart(providerProbePrompt))}, Options: chat.Options{MaxOutputTokens: &maxTokens},
	})
	return err
}

func (Capabilities) ListModels(ctx context.Context, entry provider.Provider) ([]string, error) {
	inputs, err := resolveProviderClientInputs(entry.ID(), entry)
	if err != nil {
		return nil, err
	}
	return remoteModelIDs(ctx, inputs)
}

func remoteModelIDs(ctx context.Context, inputs providerClientInputs) ([]string, error) {
	baseURL, hasBaseURL := inputs.endpoint()
	if !hasBaseURL {
		return nil, fmt.Errorf("%w: provider %q", modelsapp.ErrProviderBaseURLRequired, inputs.entry.ID())
	}
	spec, err := inputs.clientSpec(configurationProbeModel)
	if err != nil {
		if errors.Is(err, ErrCredentialUnavailable) {
			return nil, fmt.Errorf("%w: provider %q: %w", modelsapp.ErrProviderUnconfigured, inputs.entry.ID(), err)
		}
		return nil, err
	}
	if _, _, err := llm.BuildChat(spec); err != nil {
		return nil, err
	}
	return inputs.profile.ListModels(ctx, baseURL, inputs.apiKey())
}

func providerMetadata(value llm.ProviderProfile) modelsapp.ProviderMetadata {
	endpoint := modelsapp.ProviderEndpointOptional
	if value.RequiresConfiguredEndpoint() {
		endpoint = modelsapp.ProviderEndpointRequired
	}
	modelSource := modelsapp.ProviderModelsBundled
	if value.DiscoversModelsAtEndpoint() {
		modelSource = modelsapp.ProviderModelsEndpoint
	}
	embedding := modelsapp.NoEmbeddingCapability()
	if value.SupportsEmbeddings() {
		if defaultModel, present := value.DefaultEmbeddingModel(); present {
			var err error
			embedding, err = modelsapp.EmbeddingCapabilityWithDefault(defaultModel)
			if err != nil {
				panic(fmt.Sprintf("model: invalid embedding default for %q: %v", value.ID(), err))
			}
		} else {
			embedding = modelsapp.EmbeddingCapabilityWithoutDefault()
		}
	}
	authentication := modelsapp.ProviderAPIKeyOptional
	if value.RequiresAPIKey() {
		authentication = modelsapp.ProviderAPIKeyRequired
	}
	metadata, err := modelsapp.NewProviderMetadata(string(value.ID()), authentication, endpoint, modelSource, embedding)
	if err != nil {
		panic(fmt.Sprintf("model: invalid provider metadata for %q: %v", value.ID(), err))
	}
	return metadata
}

func catalogModel(providerID string, entry catalog.Model) modelsapp.Model {
	tokenLimits, err := catalogTokenLimits(entry)
	if err != nil {
		// The bundled catalog is static reference data, not user input. An invalid
		// envelope is a broken application dependency and must fail loudly instead
		// of publishing invented or partially discarded capability facts.
		panic(fmt.Sprintf("model: invalid bundled limits for %q/%q: %v", providerID, entry.ID, err))
	}
	details := &modelsapp.Details{
		DisplayName: entry.DisplayName, TokenLimits: tokenLimits, KnowledgeCutoff: entry.KnowledgeCutoff, Deprecated: entry.Deprecated,
		Reasoning: entry.Reasoning.Supported, ReasoningLevels: slices.Clone(entry.Reasoning.Levels), ReasoningDefault: entry.Reasoning.DefaultLevel,
		Multimodal: entry.Modalities.AcceptsInput(catalog.ModalityImage), InputModalities: catalogModalities(entry.Modalities.Input),
		OutputModalities: catalogModalities(entry.Modalities.Output), ToolUse: entry.ToolCall, StructuredOutput: entry.StructuredOutput,
	}
	if len(entry.Pricing) > 0 {
		primary := entry.Pricing[0]
		details.Pricing = &modelsapp.Pricing{
			InputPerMillion: primary.InputPer1M, OutputPerMillion: primary.OutputPer1M,
			CacheReadPerMillion: primary.CacheReadPer1M, CacheWritePerMillion: primary.CacheWritePer1M,
		}
	}
	model, err := modelsapp.NewModel(providerID, entry.ID, details)
	if err != nil {
		panic(fmt.Sprintf("model: invalid bundled model %q/%q: %v", providerID, entry.ID, err))
	}
	return model
}

func catalogModalities(values []catalog.Modality) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = string(value)
	}
	return out
}
