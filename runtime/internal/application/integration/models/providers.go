package models

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/application/integration/secrets"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

// ProviderSummary is the application result for provider discovery and
// configuration. It intentionally carries only the redacted credential view.
type ProviderSummary struct {
	ID                    string
	BaseURL               *string
	Credential            *ProviderCredentialSummary
	Configured            bool
	RequiresAPIKey        bool
	RequiresBaseURL       bool
	EmbeddingCapable      bool
	DefaultEmbeddingModel *string
}

// ProviderCredentialSummary is the redacted configured-credential state.
// Absence is represented by a nil summary, never by an empty mask/source.
type ProviderCredentialSummary struct {
	Masked string
	Source provider.KeySource
}

// UpdateProviderCommand carries already-validated domain changes. External
// values are parsed before this boundary; this use case owns provider policy.
type UpdateProviderCommand struct {
	ID      string
	APIKey  provider.Change[provider.APIKey]
	BaseURL provider.Change[provider.BaseURL]
}

// ProviderTestOutcome is the complete client-relevant result of a live
// credential probe. Unsupported provider remains a command error; all other
// operational details stay in observability rather than becoming arbitrary
// caller-visible error text.
type ProviderTestOutcome string

const (
	ProviderTestSucceeded     ProviderTestOutcome = "succeeded"
	ProviderTestNotConfigured ProviderTestOutcome = "not_configured"
	ProviderTestFailed        ProviderTestOutcome = "failed"
)

// ListProviders returns the supported-provider set annotated with its current
// configuration. Registry-only unknown providers are intentionally omitted.
func (c *Coordinator) ListProviders(ctx context.Context) ([]ProviderSummary, error) {
	if c.providers == nil {
		return nil, errors.New("models: provider registry is unavailable")
	}
	entries, err := c.providers.List(ctx)
	if err != nil {
		return nil, err
	}
	byID, err := indexProviderRegistry(entries)
	if err != nil {
		return nil, err
	}
	metadata, err := c.supportedProviders()
	if err != nil {
		return nil, err
	}
	out := make([]ProviderSummary, 0, len(metadata))
	for _, meta := range metadata {
		out = append(out, providerSummary(meta, byID[meta.ID()]))
	}
	return out, nil
}

// UpdateProvider validates and persists one supported provider, returning
// its redacted stored result.
func (c *Coordinator) UpdateProvider(ctx context.Context, cmd UpdateProviderCommand) (ProviderSummary, error) {
	meta, err := c.supportedProvider(cmd.ID)
	if err != nil {
		return ProviderSummary{}, err
	}
	if c.providers == nil {
		return ProviderSummary{}, errors.New("models: provider registry is unavailable")
	}
	patch := provider.Patch{APIKey: cmd.APIKey, BaseURL: cmd.BaseURL}
	if patch.Empty() {
		return ProviderSummary{}, fmt.Errorf("%w: provider %q has no changes", ErrProviderUpdateRequired, cmd.ID)
	}
	if meta.RequiresConfiguredEndpoint() {
		existing, found, getErr := c.providers.Get(ctx, cmd.ID)
		if getErr != nil {
			return ProviderSummary{}, getErr
		}
		currentBaseURL := provider.BaseURL{}
		if found {
			if err := validateProviderResult("get", cmd.ID, existing); err != nil {
				return ProviderSummary{}, err
			}
			currentBaseURL, _ = existing.BaseURL()
		}
		finalBaseURL, resolveErr := cmd.BaseURL.Resolve(currentBaseURL)
		if resolveErr != nil {
			return ProviderSummary{}, resolveErr
		}
		if !finalBaseURL.Present() {
			return ProviderSummary{}, fmt.Errorf("%w: provider %q", ErrProviderBaseURLRequired, cmd.ID)
		}
	}
	entry, err := c.providers.Update(ctx, cmd.ID, patch)
	if err != nil {
		return ProviderSummary{}, err
	}
	if err := validateProviderResult("update", cmd.ID, entry); err != nil {
		return ProviderSummary{}, err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.Models})
	return providerSummary(meta, entry), nil
}

// TestProvider checks that a supported, configured provider accepts a minimal
// request. Its result is deliberately a stable use-case outcome; integration
// diagnostics never become caller-visible data.
func (c *Coordinator) TestProvider(ctx context.Context, id string) (ProviderTestOutcome, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	_, entry, err := c.configuredProvider(ctx, id)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return "", contextErr
		}
		if errors.Is(err, ErrProviderUnsupported) {
			return "", err
		}
		if errors.Is(err, ErrProviderUnconfigured) {
			return ProviderTestNotConfigured, nil
		}
		return "", err
	}
	if c.prober == nil {
		trace.SpanFromContext(ctx).RecordError(errors.New("models: provider probe is unavailable"))
		return ProviderTestFailed, nil
	}
	probeContext, cancelProbe := context.WithTimeout(ctx, c.probeTimeout)
	defer cancelProbe()
	probeErr := c.prober.Probe(probeContext, entry)
	if contextErr := ctx.Err(); contextErr != nil {
		return "", contextErr
	}
	if probeErr != nil {
		trace.SpanFromContext(ctx).RecordError(probeErr)
		return ProviderTestFailed, nil
	}
	return ProviderTestSucceeded, nil
}

// ListModels applies the model-discovery policy for one supported provider and
// returns one entry per model ID in ascending ID order. Providers with
// endpoint-owned model sets prefer a successful non-empty remote list; every
// other outcome falls back to the static catalog, so restart behavior never
// depends on an in-memory probe result.
func (c *Coordinator) ListModels(ctx context.Context, providerID string) ([]Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	meta, err := c.supportedProvider(providerID)
	if err != nil {
		return nil, err
	}
	if meta.DiscoversModelsAtEndpoint() && c.lister != nil {
		entry, err := c.modelDiscoveryProvider(ctx, providerID)
		if err != nil {
			return nil, err
		}
		ids, probeErr := c.lister.ListModels(ctx, entry)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if probeErr == nil && len(ids) > 0 {
			out := make([]Model, 0, len(ids))
			for _, id := range ids {
				if model, ok := c.lookupModel(providerID, id); ok {
					out = append(out, model)
					continue
				}
				model, modelErr := NewModel(providerID, id, nil)
				if modelErr != nil {
					return c.catalogModels(providerID)
				}
				out = append(out, model)
			}
			return orderedModels(providerID, out)
		}
	}
	return c.catalogModels(providerID)
}

func (c *Coordinator) supportedProviders() ([]ProviderMetadata, error) {
	if c.catalog == nil {
		return nil, nil
	}
	metadata := slices.Clone(c.catalog.Supported())
	seen := make(map[string]struct{}, len(metadata))
	for index, value := range metadata {
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("models: supported provider %d is invalid: %w", index+1, err)
		}
		if _, duplicate := seen[value.ID()]; duplicate {
			return nil, fmt.Errorf("models: supported provider catalog repeats %q", value.ID())
		}
		seen[value.ID()] = struct{}{}
	}
	slices.SortFunc(metadata, func(first, second ProviderMetadata) int {
		return cmp.Compare(first.ID(), second.ID())
	})
	return metadata, nil
}

func (c *Coordinator) providerMetadata(id string) (ProviderMetadata, bool) {
	if c.catalog == nil {
		return ProviderMetadata{}, false
	}
	return c.catalog.Metadata(id)
}

func (c *Coordinator) supportedProvider(id string) (ProviderMetadata, error) {
	meta, ok := c.providerMetadata(id)
	if !ok {
		return ProviderMetadata{}, fmt.Errorf("%w: provider %q", ErrProviderUnsupported, id)
	}
	if err := meta.Validate(); err != nil {
		return ProviderMetadata{}, fmt.Errorf("models: metadata for provider %q is invalid: %w", id, err)
	}
	if meta.ID() != id {
		return ProviderMetadata{}, fmt.Errorf("models: metadata lookup for provider %q returned %q", id, meta.ID())
	}
	return meta, nil
}

func (c *Coordinator) configuredProvider(ctx context.Context, id string) (ProviderMetadata, provider.Provider, error) {
	meta, err := c.supportedProvider(id)
	if err != nil {
		return ProviderMetadata{}, provider.Provider{}, err
	}
	if c.providers == nil {
		return ProviderMetadata{}, provider.Provider{}, errors.New("models: provider registry is unavailable")
	}
	entry, ok, err := c.providers.Get(ctx, id)
	if err != nil {
		return ProviderMetadata{}, provider.Provider{}, err
	}
	if !ok {
		entry, err = provider.New(id)
		if err != nil {
			return ProviderMetadata{}, provider.Provider{}, err
		}
	} else if err := validateProviderResult("get", id, entry); err != nil {
		return ProviderMetadata{}, provider.Provider{}, err
	}
	if !meta.ConfigurationSatisfied(entry) {
		return ProviderMetadata{}, provider.Provider{}, fmt.Errorf("%w: provider %q", ErrProviderUnconfigured, id)
	}
	return meta, entry, nil
}

func (c *Coordinator) modelDiscoveryProvider(ctx context.Context, providerID string) (provider.Provider, error) {
	entry, err := provider.New(providerID)
	if err != nil {
		return provider.Provider{}, err
	}
	if c.providers != nil {
		configured, ok, getErr := c.providers.Get(ctx, providerID)
		if getErr != nil {
			return provider.Provider{}, getErr
		}
		if ok {
			if err := validateProviderResult("get", providerID, configured); err != nil {
				return provider.Provider{}, err
			}
			entry = configured
		}
	}
	return entry, nil
}

func indexProviderRegistry(entries []provider.Provider) (map[string]provider.Provider, error) {
	byID := make(map[string]provider.Provider, len(entries))
	for index, entry := range entries {
		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("models: provider registry row %d is invalid: %w", index+1, err)
		}
		if _, duplicate := byID[entry.ID()]; duplicate {
			return nil, fmt.Errorf("models: provider registry repeats %q", entry.ID())
		}
		byID[entry.ID()] = entry
	}
	return byID, nil
}

func validateProviderResult(operation, id string, entry provider.Provider) error {
	if err := entry.Validate(); err != nil {
		return fmt.Errorf("models: provider registry %s for %q returned an invalid aggregate: %w", operation, id, err)
	}
	if entry.ID() != id {
		return fmt.Errorf("models: provider registry %s for %q returned %q", operation, id, entry.ID())
	}
	return nil
}

func (c *Coordinator) catalogModels(providerID string) ([]Model, error) {
	if c.catalog == nil {
		return nil, nil
	}
	return orderedModels(providerID, c.catalog.Models(providerID))
}

func orderedModels(providerID string, models []Model) ([]Model, error) {
	models = slices.Clone(models)
	slices.SortFunc(models, func(first, second Model) int {
		return cmp.Compare(first.ID(), second.ID())
	})
	for index, model := range models {
		if model.Provider() != providerID {
			return nil, fmt.Errorf(
				"models: catalog for provider %q contains model %q from provider %q",
				providerID,
				model.ID(),
				model.Provider(),
			)
		}
		if index > 0 && model.ID() == models[index-1].ID() {
			return nil, fmt.Errorf("models: catalog for provider %q repeats model %q", providerID, model.ID())
		}
	}
	return models, nil
}

func (c *Coordinator) lookupModel(providerID, modelID string) (Model, bool) {
	if c.catalog == nil {
		return Model{}, false
	}
	return c.catalog.LookupModel(providerID, modelID)
}

func providerSummary(meta ProviderMetadata, entry provider.Provider) ProviderSummary {
	var baseURL *string
	if configuredBaseURL, present := entry.BaseURL(); present {
		value := configuredBaseURL.String()
		baseURL = &value
	}
	var credentialSummary *ProviderCredentialSummary
	if credential, configured := entry.Credential(); configured {
		key, _ := credential.APIKey()
		source, _ := credential.Source()
		credentialSummary = &ProviderCredentialSummary{
			Masked: secrets.Mask(key.Reveal()),
			Source: source,
		}
	}
	var defaultEmbeddingModel *string
	if value, present := meta.Embedding().DefaultModel(); present {
		defaultEmbeddingModel = &value
	}
	return ProviderSummary{
		ID:                    meta.ID(),
		BaseURL:               baseURL,
		Credential:            credentialSummary,
		Configured:            meta.ConfigurationSatisfied(entry),
		RequiresAPIKey:        meta.RequiresAPIKey(),
		RequiresBaseURL:       meta.RequiresConfiguredEndpoint(),
		EmbeddingCapable:      meta.Embedding().Supported(),
		DefaultEmbeddingModel: defaultEmbeddingModel,
	}
}
