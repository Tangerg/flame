package model

import (
	"context"
	"errors"
	"fmt"
	"slices"

	framecall "github.com/Tangerg/frame/call"
	"github.com/Tangerg/scope/core/chat"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	modelcatalog "github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"

	"github.com/Tangerg/scope/models/catalog"
	"github.com/samber/lo"
)

type providerCapabilities interface {
	Profiles(context.Context) ([]modelprovider.Profile, error)
	Profile(context.Context, string) (modelprovider.Profile, error)
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

func (c *Capabilities) Profiles(ctx context.Context) ([]modelapp.ProviderProfile, error) {
	profiles, err := c.adapters.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]modelapp.ProviderProfile, 0, len(profiles))
	for _, profile := range profiles {
		converted, err := projectProviderProfile(profile)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func (c *Capabilities) Profile(ctx context.Context, id string) (modelapp.ProviderProfile, error) {
	profile, err := c.adapters.Profile(ctx, id)
	if err != nil {
		return modelapp.ProviderProfile{}, err
	}
	if profile.ID != id {
		return modelapp.ProviderProfile{}, errors.New("model: adapter returned a different provider")
	}
	return projectProviderProfile(profile)
}

func projectProviderProfile(profile modelprovider.Profile) (modelapp.ProviderProfile, error) {
	if err := profile.Validate(); err != nil {
		return modelapp.ProviderProfile{}, err
	}
	meta, err := providerMetadata(profile)
	if err != nil {
		return modelapp.ProviderProfile{}, err
	}
	models := make([]modelapp.Model, 0, len(profile.Models))
	for _, entry := range profile.Models {
		model, err := catalogModel(profile.ID, entry)
		if err != nil {
			return modelapp.ProviderProfile{}, fmt.Errorf("model: catalog facts for %s/%s: %w", profile.ID, entry.ID, err)
		}
		models = append(models, model)
	}
	return modelapp.ProviderProfile{Metadata: meta, Models: models, Endpoint: profile.Endpoint}, nil
}

func (c *Capabilities) AdmitSelection(ctx context.Context, selection flamemodel.Selection) error {
	if err := selection.ValidateExact(); err != nil {
		return err
	}
	profile, err := c.adapters.Profile(ctx, selection.Provider())
	if err != nil {
		if errors.Is(err, modelapp.ErrProviderUnsupported) {
			return fmt.Errorf("%w: %w", flamemodel.ErrUnsupported, err)
		}
		return err
	}
	if profile.ID != selection.Provider() {
		return modelapp.ErrProviderUnsupported
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
	_, err = modelcatalog.NewAdmission(selection, metadata)
	return err
}

func (c *Capabilities) Probe(ctx context.Context, entry provider.Provider) error {
	if _, err := c.Profile(ctx, entry.ID()); err != nil {
		return err
	}
	connection, err := connectionFromProvider(entry.ID(), entry)
	if err != nil {
		return err
	}
	if err := c.adapters.Verify(ctx, connection); err != nil {
		if refusal, ok := errors.AsType[*framecall.Refusal](err); ok {
			switch modelprovider.FailureKind(refusal.Code) {
			case modelprovider.InvalidCredentials:
				return errors.Join(modelapp.ErrProviderCredentialsRejected, err)
			case modelprovider.Timeout:
				return errors.Join(context.DeadlineExceeded, err)
			case modelprovider.Internal, modelprovider.Canceled, modelprovider.RateLimited, modelprovider.ProviderRejected, modelprovider.ProviderUnavailable:
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
		return nil, fmt.Errorf("%w: %w", modelapp.ErrModelDiscoveryFailed, err)
	}
	for _, id := range ids {
		if _, err := flamemodel.NewModelIdentity(id); err != nil {
			return nil, err
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

func providerMetadata(value modelprovider.Profile) (modelapp.ProviderMetadata, error) {
	authentication := modelapp.ProviderAPIKeyRequired
	if value.Authentication == modelprovider.KeyOptional {
		authentication = modelapp.ProviderAPIKeyOptional
	}
	endpoint := modelapp.ProviderEndpointOptional
	if value.EndpointRequired {
		endpoint = modelapp.ProviderEndpointRequired
	}
	source := modelapp.ProviderModelsBundled
	if value.ModelSource == modelprovider.EndpointModels {
		source = modelapp.ProviderModelsEndpoint
	}
	embedding := modelapp.NoEmbeddingCapability()
	if value.Embedding != nil {
		if value.Embedding.DefaultModel != "" {
			var err error
			embedding, err = modelapp.EmbeddingCapabilityWithDefault(value.Embedding.DefaultModel)
			if err != nil {
				return modelapp.ProviderMetadata{}, err
			}
		} else {
			embedding = modelapp.EmbeddingCapabilityWithoutDefault()
		}
	}
	return modelapp.NewProviderMetadata(value.ID, authentication, endpoint, source, embedding)
}

func catalogModel(providerID string, entry catalog.Model) (modelapp.Model, error) {
	tokenLimits, err := catalogTokenLimits(entry)
	if err != nil {
		return modelapp.Model{}, err
	}
	details := &modelapp.Details{
		DisplayName: entry.DisplayName, TokenLimits: tokenLimits, KnowledgeCutoff: entry.KnowledgeCutoff, Deprecated: entry.Deprecated,
		Reasoning: entry.Reasoning.Supported, ReasoningLevels: slices.Clone(entry.Reasoning.Levels), ReasoningDefault: entry.Reasoning.DefaultLevel,
		Multimodal: entry.Modalities.AcceptsInput(catalog.ModalityImage), InputModalities: slices.Clone(entry.Modalities.Input),
		OutputModalities: slices.Clone(entry.Modalities.Output), ToolUse: entry.ToolCall, StructuredOutput: entry.StructuredOutput,
	}
	for _, band := range entry.Pricing {
		priced, err := catalogPriceBand(band)
		if err != nil {
			return modelapp.Model{}, err
		}
		details.Pricing = append(details.Pricing, priced)
	}
	return modelapp.NewModel(providerID, entry.ID, details)
}

func catalogPriceBand(band catalog.Pricing) (modelapp.PriceBand, error) {
	const million = 1_000_000
	price := modelapp.PriceBand{AboveInputTokens: band.Threshold}
	for _, rate := range []struct {
		target *float64
		usage  chat.Usage
	}{
		{&price.InputPerMillion, chat.Usage{InputTokens: million}},
		{&price.OutputPerMillion, chat.Usage{OutputTokens: million}},
		{&price.CacheReadPerMillion, chat.Usage{InputTokens: million, CacheReadInputTokens: new(int64(million))}},
		{&price.CacheWritePerMillion, chat.Usage{InputTokens: million, CacheWriteInputTokens: new(int64(million))}},
	} {
		value, err := band.Cost(rate.usage)
		if err != nil {
			return modelapp.PriceBand{}, fmt.Errorf("model catalog price: %w", err)
		}
		*rate.target = value
	}
	return price, nil
}
