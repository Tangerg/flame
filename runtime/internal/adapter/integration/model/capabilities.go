package model

import (
	"context"
	"errors"
	"fmt"
	"slices"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	modelcatalog "github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/qimu/portable"
	modelsapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	catalog "github.com/Tangerg/scope/models/catalog"
	"github.com/samber/lo"
)

type providerCapabilities interface {
	Profiles(context.Context) ([]modelprovider.Profile, error)
	RemoteModels(context.Context, modelprovider.Connection) ([]string, error)
	Verify(context.Context, modelprovider.Connection) error
}

type Capabilities struct {
	adapters providerCapabilities
}

func NewCapabilities(adapters providerCapabilities) (*Capabilities, error) {
	if lo.IsNil(adapters) {
		return nil, errors.New("model: installed provider adapters are required")
	}
	return &Capabilities{adapters: adapters}, nil
}

func (c *Capabilities) Profiles(ctx context.Context) ([]modelsapp.ProviderProfile, error) {
	profiles, err := c.adapters.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]modelsapp.ProviderProfile, 0, len(profiles))
	for _, profile := range profiles {
		if err := profile.Validate(); err != nil {
			return nil, err
		}
		meta, err := providerMetadata(profile)
		if err != nil {
			return nil, err
		}
		models := make([]modelsapp.Model, 0, len(profile.Models))
		for _, entry := range profile.Models {
			model, err := catalogModel(profile.ID, entry)
			if err != nil {
				return nil, fmt.Errorf("model: catalog facts for %s/%s: %w", profile.ID, entry.ID, err)
			}
			models = append(models, model)
		}
		result = append(result, modelsapp.ProviderProfile{Metadata: meta, Models: models})
	}
	return result, nil
}

func (c *Capabilities) AdmitSelection(ctx context.Context, selection modelchoice.Selection) error {
	if err := selection.ValidateExact(); err != nil {
		return err
	}
	profiles, err := c.adapters.Profiles(ctx)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.ID != selection.Provider() {
			continue
		}
		if err := profile.Validate(); err != nil {
			return err
		}
		var metadata *catalog.Model
		for _, entry := range profile.Models {
			if entry.ID == selection.Model() {
				owned := entry.Clone()
				metadata = &owned
				break
			}
		}
		_, err := modelcatalog.NewAdmission(selection, metadata)
		return err
	}
	return fmt.Errorf("%w: %w: %s", modelchoice.ErrUnsupported, modelsapp.ErrProviderUnsupported, selection.Provider())
}

func (c *Capabilities) Probe(ctx context.Context, entry provider.Provider) error {
	profiles, err := c.adapters.Profiles(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(profiles, func(profile modelprovider.Profile) bool { return profile.ID == entry.ID() }) {
		return modelsapp.ErrProviderUnsupported
	}
	connection, err := connectionFromProvider(entry.ID(), entry)
	if err != nil {
		return err
	}
	if err := c.adapters.Verify(ctx, connection); err != nil {
		if refusal, ok := errors.AsType[*portable.Refusal](err); ok {
			switch modelprovider.FailureKind(refusal.Code) {
			case modelprovider.InvalidCredentials:
				return errors.Join(modelsapp.ErrProviderCredentialsRejected, err)
			case modelprovider.Timeout:
				return errors.Join(context.DeadlineExceeded, err)
			}
		}
		return err
	}
	return nil
}

func (c *Capabilities) ListModels(ctx context.Context, entry provider.Provider) ([]string, error) {
	connection, err := connectionFromProvider(entry.ID(), entry)
	if err != nil {
		return nil, err
	}
	ids, err := c.adapters.RemoteModels(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", modelsapp.ErrModelDiscoveryFailed, err)
	}
	for _, id := range ids {
		if _, err := modelchoice.NewModelIdentity(id); err != nil {
			return nil, err
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

func providerMetadata(value modelprovider.Profile) (modelsapp.ProviderMetadata, error) {
	authentication := modelsapp.ProviderAPIKeyRequired
	if value.Authentication == modelprovider.KeyOptional {
		authentication = modelsapp.ProviderAPIKeyOptional
	}
	endpoint := modelsapp.ProviderEndpointOptional
	if value.EndpointRequired {
		endpoint = modelsapp.ProviderEndpointRequired
	}
	source := modelsapp.ProviderModelsBundled
	if value.ModelSource == modelprovider.EndpointModels {
		source = modelsapp.ProviderModelsEndpoint
	}
	embedding := modelsapp.NoEmbeddingCapability()
	if value.Embedding != nil {
		if value.Embedding.DefaultModel != "" {
			var err error
			embedding, err = modelsapp.EmbeddingCapabilityWithDefault(value.Embedding.DefaultModel)
			if err != nil {
				return modelsapp.ProviderMetadata{}, err
			}
		} else {
			embedding = modelsapp.EmbeddingCapabilityWithoutDefault()
		}
	}
	return modelsapp.NewProviderMetadata(value.ID, authentication, endpoint, source, embedding)
}

func catalogModel(providerID string, entry catalog.Model) (modelsapp.Model, error) {
	tokenLimits, err := catalogTokenLimits(entry)
	if err != nil {
		return modelsapp.Model{}, err
	}
	details := &modelsapp.Details{
		DisplayName: entry.DisplayName, TokenLimits: tokenLimits, KnowledgeCutoff: entry.KnowledgeCutoff, Deprecated: entry.Deprecated,
		Reasoning: entry.Reasoning.Supported, ReasoningLevels: slices.Clone(entry.Reasoning.Levels), ReasoningDefault: entry.Reasoning.DefaultLevel,
		Multimodal: entry.Modalities.AcceptsInput(catalog.ModalityImage), InputModalities: catalogModalities(entry.Modalities.Input),
		OutputModalities: catalogModalities(entry.Modalities.Output), ToolUse: entry.ToolCall, StructuredOutput: entry.StructuredOutput,
	}
	for _, band := range entry.Pricing {
		priced, err := catalogPriceBand(band)
		if err != nil {
			return modelsapp.Model{}, err
		}
		details.Pricing = append(details.Pricing, priced)
	}
	return modelsapp.NewModel(providerID, entry.ID, details)
}

// catalogPriceBand reads each effective rate as the cost the catalog bills for
// one million tokens of that kind, so the displayed card cannot disagree with
// billing about defaults such as an absent cache rate.
func catalogPriceBand(band catalog.Pricing) (modelsapp.PriceBand, error) {
	const million = 1_000_000
	rates := make([]float64, 4)
	for index, usage := range []catalog.Usage{
		{InputTokens: million},
		{OutputTokens: million},
		{InputTokens: million, CacheReadInputTokens: million},
		{InputTokens: million, CacheWriteInputTokens: million},
	} {
		rate, err := band.Cost(usage)
		if err != nil {
			return modelsapp.PriceBand{}, fmt.Errorf("model catalog price: %w", err)
		}
		rates[index] = rate
	}
	return modelsapp.PriceBand{
		AboveInputTokens: band.Threshold, InputPerMillion: rates[0], OutputPerMillion: rates[1],
		CacheReadPerMillion: rates[2], CacheWritePerMillion: rates[3],
	}, nil
}

func catalogModalities(values []catalog.Modality) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}
