// Package providerregistry decorates the model-provider registry with
// process-environment credential fallback and accurate credential provenance.
package providerregistry

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/models"
	"github.com/Tangerg/flame/runtime/internal/domain/provider"
)

var ErrIdentityMismatch = errors.New("provider registry: stored identity does not match lookup identity")

// environmentRegistry is the sole owner of the stored-over-environment
// precedence rule. Environment credentials are an immutable startup snapshot
// and can never cross the durable registry boundary.
type environmentRegistry struct {
	inner   models.ProviderRegistry
	envKeys map[string]provider.APIKey
}

// WithEnvironmentKeys validates and snapshots environment credentials before
// exposing the effective registry. Invalid host input fails composition instead
// of becoming a partially configured provider later in a Run.
func WithEnvironmentKeys(inner models.ProviderRegistry, envKeys map[string]string) (models.ProviderRegistry, error) {
	snapshot := make(map[string]provider.APIKey, len(envKeys))
	for id, rawKey := range envKeys {
		if _, err := provider.New(id); err != nil {
			return nil, fmt.Errorf("provider registry: environment provider id %q: %w", id, err)
		}
		key, err := provider.NewAPIKey(rawKey)
		if err != nil {
			return nil, fmt.Errorf("provider registry: environment credential for %q: %w", id, err)
		}
		snapshot[id] = key
	}
	return &environmentRegistry{inner: inner, envKeys: snapshot}, nil
}

func (e *environmentRegistry) resolve(entry provider.Provider, found bool, id string) (provider.Provider, bool, error) {
	if found && entry.ID() != id {
		return provider.Provider{}, false, fmt.Errorf("%w: got %q for %q", ErrIdentityMismatch, entry.ID(), id)
	}
	if !found {
		key, hasEnvironmentCredential := e.envKeys[id]
		if !hasEnvironmentCredential {
			return provider.Provider{}, false, nil
		}
		var err error
		entry, err = provider.New(id)
		if err != nil {
			return provider.Provider{}, false, err
		}
		return entry.WithEnvironmentFallback(key), true, nil
	}
	if key, hasEnvironmentCredential := e.envKeys[id]; hasEnvironmentCredential {
		entry = entry.WithEnvironmentFallback(key)
	}
	return entry, true, nil
}

func (e *environmentRegistry) Get(ctx context.Context, id string) (provider.Provider, bool, error) {
	entry, found, err := e.inner.Get(ctx, id)
	if err != nil {
		return provider.Provider{}, false, err
	}
	return e.resolve(entry, found, id)
}

func (e *environmentRegistry) List(ctx context.Context) ([]provider.Provider, error) {
	stored, err := e.inner.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Provider, 0, len(stored)+len(e.envKeys))
	seen := make(map[string]struct{}, len(stored))
	for _, entry := range stored {
		id := entry.ID()
		resolved, _, resolveErr := e.resolve(entry, true, id)
		if resolveErr != nil {
			return nil, resolveErr
		}
		out = append(out, resolved)
		seen[id] = struct{}{}
	}
	for id, key := range e.envKeys {
		if _, exists := seen[id]; exists {
			continue
		}
		entry, newErr := provider.New(id)
		if newErr != nil {
			return nil, newErr
		}
		out = append(out, entry.WithEnvironmentFallback(key))
	}
	slices.SortFunc(out, func(a, b provider.Provider) int { return cmp.Compare(a.ID(), b.ID()) })
	return out, nil
}

// Update delegates the persisted mutation before applying the environment
// overlay to its result. The overlay is never passed into the inner registry.
func (e *environmentRegistry) Update(ctx context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	entry, err := e.inner.Update(ctx, id, patch)
	if err != nil {
		return provider.Provider{}, err
	}
	resolved, _, err := e.resolve(entry, true, id)
	return resolved, err
}
