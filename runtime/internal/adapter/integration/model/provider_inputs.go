package model

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

var ErrCredentialUnavailable = errors.New("model: provider credential is unavailable")

// providerClientInputs is the adapter-owned construction boundary. It keeps
// validated domain values intact until the exact call that needs primitives.
type providerClientInputs struct {
	entry   provider.Provider
	profile ProviderProfile
}

func resolveProviderClientInputs(expectedID string, entry provider.Provider) (providerClientInputs, error) {
	if entry.ID() != expectedID {
		return providerClientInputs{}, fmt.Errorf("model: provider registry identity mismatch: got %q for %q", entry.ID(), expectedID)
	}
	profile, found := LookupProvider(Provider(expectedID))
	if !found {
		return providerClientInputs{}, fmt.Errorf("model: unsupported provider %q", expectedID)
	}
	return providerClientInputs{entry: entry, profile: profile}, nil
}

func (i providerClientInputs) clientSpec() (clientSpec, error) {
	apiKey, configured := i.entry.APIKey()
	if !configured {
		return clientSpec{}, ErrCredentialUnavailable
	}
	spec, err := newClientSpec(i.profile.ID(), apiKey)
	if err != nil {
		return clientSpec{}, err
	}
	baseURL, configured := i.entry.BaseURL()
	if !configured {
		return spec, nil
	}
	return spec.WithBaseURL(baseURL.String())
}

func (i providerClientInputs) endpoint() (string, bool) {
	if baseURL, configured := i.entry.BaseURL(); configured {
		return baseURL.String(), true
	}
	return "", false
}

func (i providerClientInputs) embeddingSpaceID(model string) string {
	baseURL, _ := i.entry.BaseURL()
	return embeddingSpaceID(string(i.profile.ID()), model, baseURL.String())
}
