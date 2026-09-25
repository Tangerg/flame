package testsupport

import (
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

// MustProvider builds a configured provider entry from its raw parts. An empty
// key or base URL leaves that part unconfigured, which is how a fixture asks
// for a provider that has not been given one; anything else that fails to
// validate is a broken fixture and panics like the rest of this package.
func MustProvider(id, rawKey, rawBaseURL string) provider.Provider {
	entry, err := provider.New(id)
	if err != nil {
		panic(err)
	}
	patch := provider.Patch{}
	if rawKey != "" {
		key, keyErr := provider.NewAPIKey(rawKey)
		if keyErr != nil {
			panic(keyErr)
		}
		patch.APIKey = provider.Set(key)
	}
	if rawBaseURL != "" {
		baseURL, baseURLErr := provider.NewBaseURL(rawBaseURL)
		if baseURLErr != nil {
			panic(baseURLErr)
		}
		patch.BaseURL = provider.Set(baseURL)
	}
	entry, err = entry.Apply(patch)
	if err != nil {
		panic(err)
	}
	return entry
}
