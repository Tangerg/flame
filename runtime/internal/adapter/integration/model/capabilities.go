package model

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	modelcatalog "github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	modelsapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
	catalog "github.com/Tangerg/scope/models/catalog"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type providerCapabilities interface {
	Profiles(context.Context) ([]modelprovider.Profile, error)
	RemoteModels(context.Context, modelprovider.Connection) ([]string, error)
	OpenChat(context.Context, modelchoice.Selection, modelprovider.Connection) (ResolvedChat, error)
}

type Capabilities struct {
	adapters providerCapabilities
	journal  endpointJournal
}

func NewCapabilities(adapters providerCapabilities, journal endpointJournal) (*Capabilities, error) {
	if lo.IsNil(adapters) || lo.IsNil(journal) {
		return nil, errors.New("model: installed provider adapters and endpoint evidence are required")
	}
	return &Capabilities{adapters: adapters, journal: journal}, nil
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

func (c *Capabilities) Probe(ctx context.Context, entry provider.Provider) (returned error) {
	profiles, err := c.adapters.Profiles(ctx)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(profiles, func(profile modelprovider.Profile) bool { return profile.ID == entry.ID() })
	if index < 0 {
		return modelsapp.ErrProviderUnsupported
	}
	profile := profiles[index]
	connection, err := connectionFromProvider(entry.ID(), entry)
	if err != nil {
		return err
	}
	if profile.ModelSource == modelprovider.EndpointModels {
		ids, err := c.adapters.RemoteModels(ctx, connection)
		if err != nil {
			return probeFailure(err)
		}
		if len(ids) == 0 {
			return errors.New("model: provider advertised no models")
		}
		return nil
	}
	selection, err := modelchoice.New(profile.ID, profile.DefaultModel)
	if err != nil {
		return err
	}
	resolved, err := c.adapters.OpenChat(ctx, selection, connection)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, resolved.Close(ctx)) }()
	id := uuid.NewString()
	ctx, err = qimumcp.WithInvocation(ctx, portable.Invocation{ID: id, Origin: extension.Human, Parent: operation.Identity(ctx), Target: portable.Target{Scope: extension.ScopeCall, Reference: id}})
	if err != nil {
		return err
	}
	observed := &observedEndpoint{inner: resolved.Model(), journal: c.journal, selection: selection, pricing: resolved.pricing}
	maxTokens := int64(1)
	_, err = observed.Call(ctx, &chat.Request{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("ping"))}, Options: chat.Options{MaxOutputTokens: &maxTokens}})
	return probeFailure(err)
}

func probeFailure(err error) error {
	if failure, ok := errors.AsType[*run.FailureError](err); ok {
		switch failure.Kind {
		case run.FailureInvalidCredentials:
			return errors.Join(modelsapp.ErrProviderCredentialsRejected, err)
		case run.FailureTimeout:
			return errors.Join(context.DeadlineExceeded, err)
		}
	}
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
	if len(entry.Pricing) > 0 {
		primary := entry.Pricing[0]
		details.Pricing = &modelsapp.Pricing{InputPerMillion: primary.InputPer1M, OutputPerMillion: primary.OutputPer1M, CacheReadPerMillion: lo.FromPtrOr(primary.CacheReadPer1M, primary.InputPer1M), CacheWritePerMillion: lo.FromPtrOr(primary.CacheWritePer1M, primary.InputPer1M)}
	}
	return modelsapp.NewModel(providerID, entry.ID, details)
}

func catalogModalities(values []catalog.Modality) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}
