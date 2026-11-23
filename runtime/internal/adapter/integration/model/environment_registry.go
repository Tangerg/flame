package model

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/samber/lo"
)

var errRegistryIdentityMismatch = errors.New("model: stored provider identity does not match lookup identity")

type environmentRegistry struct {
	inner   models.ProviderRegistry
	envKeys map[string]provider.APIKey
}

func WithEnvironmentKeys(inner models.ProviderRegistry, envKeys map[string]string) (models.ProviderRegistry, error) {
	if lo.IsNil(inner) {
		return nil, errors.New("model: durable provider registry is required")
	}
	snapshot := make(map[string]provider.APIKey, len(envKeys))
	for id, rawKey := range envKeys {
		if _, err := provider.New(id); err != nil {
			return nil, fmt.Errorf("model: environment provider id %q: %w", id, err)
		}
		key, err := provider.NewAPIKey(rawKey)
		if err != nil {
			return nil, fmt.Errorf("model: environment credential for %q: %w", id, err)
		}
		snapshot[id] = key
	}
	return &environmentRegistry{inner: inner, envKeys: snapshot}, nil
}

func (e *environmentRegistry) overlay(entry provider.Provider) provider.Provider {
	if key, found := e.envKeys[entry.ID()]; found {
		return entry.WithEnvironmentFallback(key)
	}
	return entry
}

func (e *environmentRegistry) addressed(entry provider.Provider, id string) (provider.Provider, error) {
	if entry.ID() != id {
		return provider.Provider{}, fmt.Errorf("%w: got %q for %q", errRegistryIdentityMismatch, entry.ID(), id)
	}
	return e.overlay(entry), nil
}

func (e *environmentRegistry) environmentOnly(id string) (provider.Provider, bool, error) {
	key, found := e.envKeys[id]
	if !found {
		return provider.Provider{}, false, nil
	}
	entry, err := provider.New(id)
	if err != nil {
		return provider.Provider{}, false, err
	}
	return entry.WithEnvironmentFallback(key), true, nil
}

func (e *environmentRegistry) Get(ctx context.Context, id string) (provider.Provider, bool, error) {
	entry, found, err := e.inner.Get(ctx, id)
	if err != nil {
		return provider.Provider{}, false, err
	}
	if !found {
		return e.environmentOnly(id)
	}
	resolved, err := e.addressed(entry, id)
	return resolved, err == nil, err
}

func (e *environmentRegistry) List(ctx context.Context) ([]provider.Provider, error) {
	stored, err := e.inner.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Provider, 0, len(stored)+len(e.envKeys))
	seen := make(map[string]struct{}, len(stored))
	for _, entry := range stored {
		out = append(out, e.overlay(entry))
		seen[entry.ID()] = struct{}{}
	}
	for id := range e.envKeys {
		if _, exists := seen[id]; exists {
			continue
		}
		entry, _, err := e.environmentOnly(id)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b provider.Provider) int { return cmp.Compare(a.ID(), b.ID()) })
	return out, nil
}

func (e *environmentRegistry) Update(ctx context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	entry, err := e.inner.Update(ctx, id, patch)
	if err != nil {
		return provider.Provider{}, err
	}
	return e.addressed(entry, id)
}
