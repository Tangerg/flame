package model

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/llm"
)

var (
	ErrProviderIdentityMismatch = errors.New("model: provider registry identity mismatch")
	ErrCredentialUnavailable    = errors.New("model: provider credential is unavailable")
)

// providerClientInputs is the adapter-owned construction boundary. It keeps
// validated domain values intact until the exact call that needs primitives.
type providerClientInputs struct {
	entry   provider.Provider
	profile llm.ProviderProfile
}

func resolveProviderClientInputs(expectedID string, entry provider.Provider) (providerClientInputs, error) {
	if entry.ID() != expectedID {
		return providerClientInputs{}, fmt.Errorf("%w: got %q for %q", ErrProviderIdentityMismatch, entry.ID(), expectedID)
	}
	profile, found := llm.LookupProvider(llm.Provider(expectedID))
	if !found {
		return providerClientInputs{}, fmt.Errorf("model: unsupported provider %q", expectedID)
	}
	return providerClientInputs{entry: entry, profile: profile}, nil
}

func (i providerClientInputs) clientSpec(model string) (llm.ClientSpec, error) {
	credential := llm.NoClientCredential()
	if apiKey, configured := i.entry.APIKey(); configured {
		var err error
		credential, err = llm.NewAPIKeyCredential(apiKey.Reveal())
		if err != nil {
			return llm.ClientSpec{}, err
		}
	} else if i.profile.RequiresAPIKey() {
		return llm.ClientSpec{}, ErrCredentialUnavailable
	}
	spec, err := llm.NewClientSpec(i.profile.ID(), model, credential)
	if err != nil {
		return llm.ClientSpec{}, err
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
	return i.profile.DefaultEndpoint()
}

func (i providerClientInputs) apiKey() string {
	apiKey, configured := i.entry.APIKey()
	if !configured {
		return ""
	}
	return apiKey.Reveal()
}

func (i providerClientInputs) embeddingSpaceID(model string) string {
	baseURL, _ := i.entry.BaseURL()
	return embeddingSpaceID(string(i.profile.ID()), model, baseURL.String())
}
