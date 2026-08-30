package modelclient

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/provider"
	"github.com/Tangerg/flame/runtime/internal/infra/llm"
)

var (
	ErrProviderIdentityMismatch = errors.New("modelclient: provider registry identity mismatch")
	ErrCredentialUnavailable    = errors.New("modelclient: provider credential is unavailable")
)

// providerClientInputs is the adapter-owned construction boundary. It keeps
// validated domain values intact until the exact call that needs primitives.
type providerClientInputs struct {
	credential llm.ClientCredential
	baseURL    provider.BaseURL
	hasBaseURL bool
}

func resolveProviderClientInputs(expectedID string, entry provider.Provider) (providerClientInputs, error) {
	if entry.ID() != expectedID {
		return providerClientInputs{}, fmt.Errorf("%w: got %q for %q", ErrProviderIdentityMismatch, entry.ID(), expectedID)
	}
	profile, found := llm.LookupProvider(llm.Provider(expectedID))
	if !found {
		return providerClientInputs{}, fmt.Errorf("modelclient: unsupported provider %q", expectedID)
	}
	credential := llm.NoClientCredential()
	if apiKey, configured := entry.APIKey(); configured {
		var err error
		credential, err = llm.NewAPIKeyCredential(apiKey.Reveal())
		if err != nil {
			return providerClientInputs{}, err
		}
	} else if profile.RequiresAPIKey() {
		return providerClientInputs{}, ErrCredentialUnavailable
	}
	baseURL, hasBaseURL := entry.BaseURL()
	return providerClientInputs{
		credential: credential, baseURL: baseURL, hasBaseURL: hasBaseURL,
	}, nil
}

func (i providerClientInputs) clientSpec(providerID, model string) (llm.ClientSpec, error) {
	spec, err := llm.NewClientSpec(llm.Provider(providerID), model, i.credential)
	if err != nil {
		return llm.ClientSpec{}, err
	}
	if !i.hasBaseURL {
		return spec, nil
	}
	return spec.WithBaseURL(i.baseURL.String())
}

func (i providerClientInputs) embeddingSpaceID(providerID, model string) string {
	return embeddingSpaceID(providerID, model, i.baseURL.String())
}
