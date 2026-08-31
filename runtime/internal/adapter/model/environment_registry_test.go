package model

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

type fakeRegistry struct {
	stored map[string]provider.Provider
}

func (f *fakeRegistry) List(context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(f.stored))
	for _, entry := range f.stored {
		out = append(out, entry)
	}
	return out, nil
}

func (f *fakeRegistry) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	entry, found := f.stored[id]
	return entry, found, nil
}

func (f *fakeRegistry) Update(_ context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	entry, found := f.stored[id]
	if !found {
		var err error
		entry, err = provider.New(id)
		if err != nil {
			return provider.Provider{}, err
		}
	}
	updated, err := entry.Apply(patch)
	if err != nil {
		return provider.Provider{}, err
	}
	f.stored[id] = updated
	return updated, nil
}

func configuredProvider(t *testing.T, id, rawKey, rawBaseURL string) provider.Provider {
	t.Helper()
	entry, err := provider.New(id)
	if err != nil {
		t.Fatal(err)
	}
	patch := provider.Patch{}
	if rawKey != "" {
		key, keyErr := provider.NewAPIKey(rawKey)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		patch.APIKey = provider.Set(key)
	}
	if rawBaseURL != "" {
		baseURL, baseURLErr := provider.NewBaseURL(rawBaseURL)
		if baseURLErr != nil {
			t.Fatal(baseURLErr)
		}
		patch.BaseURL = provider.Set(baseURL)
	}
	entry, err = entry.Apply(patch)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func providerCredential(t *testing.T, entry provider.Provider) (string, provider.KeySource) {
	t.Helper()
	credential, configured := entry.Credential()
	if !configured {
		t.Fatal("provider is not configured")
	}
	key, _ := credential.APIKey()
	source, _ := credential.Source()
	return key.Reveal(), source
}

func registryWithEnvironment(t *testing.T, inner *fakeRegistry, envKeys map[string]string) interface {
	Get(context.Context, string) (provider.Provider, bool, error)
	List(context.Context) ([]provider.Provider, error)
	Update(context.Context, string, provider.Patch) (provider.Provider, error)
} {
	t.Helper()
	registry, err := WithEnvironmentKeys(inner, envKeys)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestStoredCredentialWinsEnvironmentFallback(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"anthropic": configuredProvider(t, "anthropic", "sk-stored", "https://x"),
	}}
	registry := registryWithEnvironment(t, inner, map[string]string{"anthropic": "sk-env"})

	got, found, err := registry.Get(t.Context(), "anthropic")
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	key, source := providerCredential(t, got)
	baseURL, present := got.BaseURL()
	if key != "sk-stored" || source != provider.KeyStored || !present || baseURL.String() != "https://x" {
		t.Fatalf("provider = (%q, %q, %q, %v)", key, source, baseURL.String(), present)
	}
}

func TestEnvironmentOnlyProviderIsEnabled(t *testing.T) {
	registry := registryWithEnvironment(t, &fakeRegistry{stored: map[string]provider.Provider{}}, map[string]string{"openai": "sk-env"})
	got, found, err := registry.Get(t.Context(), "openai")
	_, configured := got.Credential()
	if err != nil || !found || !configured {
		t.Fatalf("Get env-only: found=%v configured=%v err=%v", found, configured, err)
	}
	key, source := providerCredential(t, got)
	if key != "sk-env" || source != provider.KeyEnvironment {
		t.Fatalf("credential = (%q, %q)", key, source)
	}
}

func TestEnvironmentFallbackPreservesStoredEndpoint(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"deepseek": configuredProvider(t, "deepseek", "", "https://ep"),
	}}
	registry := registryWithEnvironment(t, inner, map[string]string{"deepseek": "sk-env"})
	got, _, err := registry.Get(t.Context(), "deepseek")
	if err != nil {
		t.Fatal(err)
	}
	key, source := providerCredential(t, got)
	baseURL, present := got.BaseURL()
	if key != "sk-env" || source != provider.KeyEnvironment || !present || baseURL.String() != "https://ep" {
		t.Fatalf("provider = (%q, %q, %q, %v)", key, source, baseURL.String(), present)
	}
}

func TestUpdateNeverPersistsEnvironmentCredential(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"deepseek": configuredProvider(t, "deepseek", "sk-stored", "https://old"),
	}}
	registry := registryWithEnvironment(t, inner, map[string]string{"deepseek": "sk-env"})
	baseURL, _ := provider.NewBaseURL("https://new")
	got, err := registry.Update(t.Context(), "deepseek", provider.Patch{BaseURL: provider.Set(baseURL)})
	if err != nil {
		t.Fatal(err)
	}
	effectiveKey, _ := providerCredential(t, got)
	storedKey, _ := providerCredential(t, inner.stored["deepseek"])
	if effectiveKey != "sk-stored" || storedKey != "sk-stored" {
		t.Fatalf("base URL update changed credential: effective=%q stored=%q", effectiveKey, storedKey)
	}

	got, err = registry.Update(t.Context(), "deepseek", provider.Patch{APIKey: provider.Clear[provider.APIKey]()})
	if err != nil {
		t.Fatal(err)
	}
	effectiveKey, source := providerCredential(t, got)
	if effectiveKey != "sk-env" || source != provider.KeyEnvironment {
		t.Fatalf("cleared stored key resolved as (%q, %q)", effectiveKey, source)
	}
	if _, configured := inner.stored["deepseek"].Credential(); configured {
		t.Fatal("environment credential crossed durable registry boundary")
	}
}

func TestListMergesEnvironmentOnlyProvidersAndSorts(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"openai": configuredProvider(t, "openai", "sk-stored", ""),
	}}
	registry := registryWithEnvironment(t, inner, map[string]string{
		"anthropic": "sk-env",
		"openai":    "sk-env",
	})
	list, err := registry.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID() != "anthropic" || list[1].ID() != "openai" {
		t.Fatalf("providers = %+v", list)
	}
	_, firstSource := providerCredential(t, list[0])
	_, secondSource := providerCredential(t, list[1])
	if firstSource != provider.KeyEnvironment || secondSource != provider.KeyStored {
		t.Fatalf("sources = (%q, %q)", firstSource, secondSource)
	}
}

func TestEnvironmentSnapshotRejectsInvalidInputAndIgnoresCallerMutation(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{}}
	if _, err := WithEnvironmentKeys(inner, map[string]string{"openai": ""}); !errors.Is(err, provider.ErrAPIKeyRequired) {
		t.Fatalf("blank environment credential error = %v", err)
	}
	environment := map[string]string{"openai": "before"}
	registry := registryWithEnvironment(t, inner, environment)
	environment["openai"] = "after"
	got, found, err := registry.Get(t.Context(), "openai")
	if err != nil || !found {
		t.Fatalf("Get after caller mutation: found=%v err=%v", found, err)
	}
	key, _ := providerCredential(t, got)
	if key != "before" {
		t.Fatalf("environment key = %q, want startup snapshot", key)
	}
}

func TestRegistryRejectsMismatchedStoredIdentity(t *testing.T) {
	inner := &fakeRegistry{stored: map[string]provider.Provider{
		"openai": configuredProvider(t, "anthropic", "sk", ""),
	}}
	registry := registryWithEnvironment(t, inner, nil)
	_, _, err := registry.Get(t.Context(), "openai")
	if !errors.Is(err, ErrRegistryIdentityMismatch) {
		t.Fatalf("identity mismatch error = %v", err)
	}
}
