package models

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

type testProviderRegistry struct {
	entries   map[string]provider.Provider
	updates   []provider.Patch
	getErr    error
	updateErr error
}

func (t *testProviderRegistry) List(context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(t.entries))
	for _, entry := range t.entries {
		out = append(out, entry)
	}
	return out, nil
}

func (t *testProviderRegistry) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	if t.getErr != nil {
		return provider.Provider{}, false, t.getErr
	}
	entry, ok := t.entries[id]
	return entry, ok, nil
}

func (t *testProviderRegistry) Update(_ context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	if t.updateErr != nil {
		return provider.Provider{}, t.updateErr
	}
	t.updates = append(t.updates, patch)
	if t.entries == nil {
		t.entries = map[string]provider.Provider{}
	}
	entry, found := t.entries[id]
	if !found {
		var err error
		entry, err = provider.New(id)
		if err != nil {
			return provider.Provider{}, err
		}
	}
	entry, err := entry.Apply(patch)
	if err != nil {
		return provider.Provider{}, err
	}
	t.entries[id] = entry
	return entry, nil
}

func modelProvider(t *testing.T, id, rawKey, rawBaseURL string) provider.Provider {
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

type testCatalog struct {
	metadata []ProviderMetadata
	models   map[string][]Model
}

func (t testCatalog) Supported() []ProviderMetadata { return slices.Clone(t.metadata) }

func (t testCatalog) Metadata(id string) (ProviderMetadata, bool) {
	for _, metadata := range t.metadata {
		if metadata.ID() == id {
			return metadata, true
		}
	}
	return ProviderMetadata{}, false
}

func providerMetadataFixture(t testing.TB, id string, endpoint ProviderEndpointPolicy, source ProviderModelSource, embedding EmbeddingCapability) ProviderMetadata {
	t.Helper()
	metadata, err := NewProviderMetadata(id, ProviderAPIKeyRequired, endpoint, source, embedding)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func optionalAPIKeyProviderMetadataFixture(t testing.TB, id string, endpoint ProviderEndpointPolicy, source ProviderModelSource, embedding EmbeddingCapability) ProviderMetadata {
	t.Helper()
	metadata, err := NewProviderMetadata(id, ProviderAPIKeyOptional, endpoint, source, embedding)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func (t testCatalog) Models(providerID string) []Model {
	return slices.Clone(t.models[providerID])
}

func (t testCatalog) LookupModel(providerID, modelID string) (Model, bool) {
	for _, model := range t.models[providerID] {
		if model.ID() == modelID {
			return model, true
		}
	}
	return Model{}, false
}

func TestListProvidersOwnsCatalogOrder(t *testing.T) {
	c := New(Config{
		Providers: &testProviderRegistry{},
		Catalog: testCatalog{metadata: []ProviderMetadata{
			providerMetadataFixture(t, "zeta", ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability()),
			providerMetadataFixture(t, "alpha", ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability()),
		}},
	})

	providers, err := c.ListProviders(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 || providers[0].ID != "alpha" || providers[1].ID != "zeta" {
		t.Fatalf("providers = %+v, want alpha then zeta", providers)
	}
}

func TestListModelsOwnsCatalogOrderAndSnapshot(t *testing.T) {
	providerID := "static"
	catalog := testCatalog{
		metadata: []ProviderMetadata{providerMetadataFixture(
			t, providerID, ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability(),
		)},
		models: map[string][]Model{providerID: {
			catalogModelFixture(t, providerID, "zeta", &Details{DisplayName: "Zeta"}),
			catalogModelFixture(t, providerID, "alpha", &Details{DisplayName: "Alpha"}),
		}},
	}
	c := New(Config{Catalog: catalog})

	models, err := c.ListModels(t.Context(), providerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID() != "alpha" || models[1].ID() != "zeta" {
		t.Fatalf("models = %+v, want alpha then zeta", models)
	}
	if catalog.models[providerID][0].ID() != "zeta" {
		t.Fatal("ListModels mutated catalog storage while ordering its result")
	}
}

func TestListModelsRejectsContradictoryCatalogIdentity(t *testing.T) {
	providerID := "static"
	alpha := catalogModelFixture(t, providerID, "alpha", &Details{})
	for name, entries := range map[string][]Model{
		"duplicate model":  {alpha, alpha},
		"foreign provider": {catalogModelFixture(t, "other", "alpha", &Details{})},
	} {
		t.Run(name, func(t *testing.T) {
			c := New(Config{Catalog: testCatalog{
				metadata: []ProviderMetadata{providerMetadataFixture(
					t, providerID, ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability(),
				)},
				models: map[string][]Model{providerID: entries},
			}})
			if models, err := c.ListModels(t.Context(), providerID); err == nil || models != nil {
				t.Fatalf("ListModels = (%+v, %v), want nil/error", models, err)
			}
		})
	}
}

func catalogModelFixture(t testing.TB, providerID, modelID string, details *Details) Model {
	t.Helper()
	model, err := NewModel(providerID, modelID, details)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

type fakeLister struct {
	gotEntry provider.Provider
	ids      []string
	err      error
}

func (f *fakeLister) ListModels(_ context.Context, entry provider.Provider) ([]string, error) {
	f.gotEntry = entry
	return f.ids, f.err
}

type fakeProber struct {
	got     provider.Provider
	err     error
	onProbe func()
}

type waitingProber struct{}

func (waitingProber) Probe(ctx context.Context, _ provider.Provider) error {
	<-ctx.Done()
	return context.Cause(ctx)
}

func TestOptionalAPIKeyProviderIsConfiguredWithoutRegistryRow(t *testing.T) {
	prober := &fakeProber{}
	c := New(Config{
		Providers: &testProviderRegistry{},
		Catalog: testCatalog{metadata: []ProviderMetadata{optionalAPIKeyProviderMetadataFixture(
			t, "ollama", ProviderEndpointOptional, ProviderModelsEndpoint, EmbeddingCapabilityWithoutDefault(),
		)}},
		Prober: prober,
	})

	providers, err := c.ListProviders(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || !providers[0].Configured || providers[0].RequiresAPIKey || providers[0].Credential != nil {
		t.Fatalf("optional provider summary = %+v", providers)
	}
	if outcome, err := c.TestProvider(t.Context(), "ollama"); err != nil || outcome != ProviderTestSucceeded {
		t.Fatalf("TestProvider = %q, %v", outcome, err)
	}
	if prober.got.ID() != "ollama" {
		t.Fatalf("probed provider = %q", prober.got.ID())
	}
}

func TestProviderProbePreservesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c := New(Config{
		Providers: &testProviderRegistry{},
		Catalog: testCatalog{metadata: []ProviderMetadata{optionalAPIKeyProviderMetadataFixture(
			t, "ollama", ProviderEndpointOptional, ProviderModelsEndpoint, NoEmbeddingCapability(),
		)}},
		Prober: &fakeProber{onProbe: cancel},
	})

	outcome, err := c.TestProvider(ctx, "ollama")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("TestProvider error = %v, want caller cancellation", err)
	}
	if outcome != "" {
		t.Fatalf("TestProvider outcome = %q, want no operational outcome after cancellation", outcome)
	}
}

func TestProviderProbeOwnsASettlementDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	c := New(Config{
		Providers: &testProviderRegistry{},
		Catalog: testCatalog{metadata: []ProviderMetadata{optionalAPIKeyProviderMetadataFixture(
			t, "ollama", ProviderEndpointOptional, ProviderModelsEndpoint, NoEmbeddingCapability(),
		)}},
		Prober:       waitingProber{},
		ProbeTimeout: 10 * time.Millisecond,
	})

	outcome, err := c.TestProvider(ctx, "ollama")
	if err != nil {
		t.Fatalf("TestProvider returned deadline as a command error: %v", err)
	}
	if outcome != ProviderTestFailed {
		t.Fatalf("TestProvider outcome = %q, want %q", outcome, ProviderTestFailed)
	}
	if ctx.Err() != nil {
		t.Fatal("provider probe consumed the caller's safety deadline")
	}
}

func TestProviderProbePreservesRegistryFailure(t *testing.T) {
	sentinel := errors.New("registry unavailable")
	c := New(Config{
		Providers: &testProviderRegistry{getErr: sentinel},
		Catalog: testCatalog{metadata: []ProviderMetadata{optionalAPIKeyProviderMetadataFixture(
			t, "ollama", ProviderEndpointOptional, ProviderModelsEndpoint, NoEmbeddingCapability(),
		)}},
		Prober: &fakeProber{},
	})

	outcome, err := c.TestProvider(t.Context(), "ollama")
	if !errors.Is(err, sentinel) {
		t.Fatalf("TestProvider error = %v, want registry failure", err)
	}
	if outcome != "" {
		t.Fatalf("TestProvider outcome = %q, want no probe outcome after registry failure", outcome)
	}
}

func (f *fakeProber) Probe(_ context.Context, entry provider.Provider) error {
	f.got = entry
	if f.onProbe != nil {
		f.onProbe()
	}
	return f.err
}

func TestListModelsPrefersRemoteModelsAndEnrichesKnownEntries(t *testing.T) {
	registry := &testProviderRegistry{entries: map[string]provider.Provider{
		"ollama": modelProvider(t, "ollama", "k", "http://host:1234/v1"),
	}}
	catalog := testCatalog{
		metadata: []ProviderMetadata{optionalAPIKeyProviderMetadataFixture(t, "ollama", ProviderEndpointOptional, ProviderModelsEndpoint, NoEmbeddingCapability())},
		models:   map[string][]Model{"ollama": {catalogModelFixture(t, "ollama", "known", &Details{DisplayName: "Known"})}},
	}
	lister := &fakeLister{ids: []string{"local", "known"}}
	c := New(Config{Providers: registry, Catalog: catalog, Lister: lister})

	got, err := c.ListModels(t.Context(), "ollama")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Details() == nil || got[0].Details().DisplayName != "Known" || got[1].ID() != "local" || got[1].Details() != nil {
		t.Fatalf("models = %+v", got)
	}
	gotBaseURL, _ := lister.gotEntry.BaseURL()
	gotKey, _ := lister.gotEntry.APIKey()
	if gotBaseURL.String() != "http://host:1234/v1" || gotKey.Reveal() != "k" {
		t.Fatalf("lister entry = (%q, %q), want configured endpoint + key", gotBaseURL.String(), gotKey.Reveal())
	}
}

func TestListModelsFallsBackToStaticCatalogWhenProbeCannotAnswer(t *testing.T) {
	catalog := testCatalog{
		metadata: []ProviderMetadata{optionalAPIKeyProviderMetadataFixture(t, "ollama", ProviderEndpointOptional, ProviderModelsEndpoint, NoEmbeddingCapability())},
		models:   map[string][]Model{"ollama": {catalogModelFixture(t, "ollama", "fallback", &Details{})}},
	}
	c := New(Config{
		Providers: &testProviderRegistry{},
		Catalog:   catalog,
		Lister:    &fakeLister{err: errors.New("offline")},
	})

	got, err := c.ListModels(t.Context(), "ollama")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID() != "fallback" {
		t.Fatalf("models = %+v, want static fallback", got)
	}
}

func TestListModelsSkipsRemoteProbeForStaticProvider(t *testing.T) {
	lister := &fakeLister{ids: []string{"must-not-appear"}}
	c := New(Config{
		Catalog: testCatalog{
			metadata: []ProviderMetadata{providerMetadataFixture(t, "anthropic", ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability())},
			models:   map[string][]Model{"anthropic": {catalogModelFixture(t, "anthropic", "cataloged", &Details{})}},
		},
		Lister: lister,
	})

	got, err := c.ListModels(t.Context(), "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID() != "cataloged" || lister.gotEntry.ID() != "" {
		t.Fatalf("models=%+v lister=%+v", got, lister.gotEntry)
	}
}

func TestListModelsRejectsUnsupportedProvider(t *testing.T) {
	lister := &fakeLister{ids: []string{"must-not-appear"}}
	c := New(Config{
		Catalog: testCatalog{metadata: []ProviderMetadata{providerMetadataFixture(
			t, "supported", ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability(),
		)}},
		Lister: lister,
	})

	models, err := c.ListModels(t.Context(), "missing")
	if !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("ListModels error = %v, want unsupported provider", err)
	}
	if models != nil || lister.gotEntry.ID() != "" {
		t.Fatalf("ListModels = %+v, lister=%+v; unsupported provider reached discovery", models, lister.gotEntry)
	}
}

func TestUpdateProviderOwnsSupportAndBaseURLPolicy(t *testing.T) {
	registry := &testProviderRegistry{}
	c := New(Config{
		Providers: registry,
		Catalog: testCatalog{metadata: []ProviderMetadata{providerMetadataFixture(
			t, "compat", ProviderEndpointRequired, ProviderModelsEndpoint, NoEmbeddingCapability(),
		)}},
	})

	apiKey, _ := provider.NewAPIKey("sk-secret")
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", APIKey: provider.Set(apiKey)}); !errors.Is(err, ErrProviderBaseURLRequired) {
		t.Fatalf("missing base URL error = %v", err)
	}
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "unknown", APIKey: provider.Set(apiKey)}); !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("unknown provider error = %v", err)
	}
	baseURL, _ := provider.NewBaseURL("https://example.test")
	configured, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", APIKey: provider.Set(apiKey), BaseURL: provider.Set(baseURL)})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(registry.updates) != 1 || configured.Credential == nil || configured.Credential.Masked == "sk-secret" {
		t.Fatalf("updated=%+v patches=%+v", configured, registry.updates)
	}

	replacement, _ := provider.NewAPIKey("sk-replaced")
	configured, err = c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", APIKey: provider.Set(replacement)})
	if err != nil {
		t.Fatalf("update key while preserving endpoint: %v", err)
	}
	if configured.BaseURL == nil || *configured.BaseURL != baseURL.String() {
		t.Fatalf("base URL = %v, want preserved %q", configured.BaseURL, baseURL.String())
	}

	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", BaseURL: provider.Clear[provider.BaseURL]()}); !errors.Is(err, ErrProviderBaseURLRequired) {
		t.Fatalf("clear required base URL error = %v", err)
	}
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat"}); !errors.Is(err, ErrProviderUpdateRequired) {
		t.Fatalf("empty update error = %v", err)
	}
}

func TestTestProviderRequiresAConfiguredSupportedProvider(t *testing.T) {
	prober := &fakeProber{}
	c := New(Config{
		Providers: &testProviderRegistry{entries: map[string]provider.Provider{
			"anthropic": modelProvider(t, "anthropic", "sk-secret", ""),
		}},
		Catalog: testCatalog{metadata: []ProviderMetadata{providerMetadataFixture(t, "anthropic", ProviderEndpointOptional, ProviderModelsBundled, NoEmbeddingCapability())}},
		Prober:  prober,
	})

	if _, err := c.TestProvider(t.Context(), "missing"); !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("unsupported error = %v", err)
	}
	if outcome, err := c.TestProvider(t.Context(), "anthropic"); err != nil || outcome != ProviderTestSucceeded {
		t.Fatalf("test provider = %q, %v", outcome, err)
	}
	if prober.got.ID() != "anthropic" {
		t.Fatalf("probed = %+v", prober.got)
	}
}

func TestCommittedProviderUpdatePublishesModelsInvalidation(t *testing.T) {
	var notices []invalidation.Notice
	c := New(Config{
		Providers: &testProviderRegistry{},
		Catalog: testCatalog{metadata: []ProviderMetadata{providerMetadataFixture(
			t, "compat", ProviderEndpointRequired, ProviderModelsEndpoint, NoEmbeddingCapability(),
		)}},
		Invalidations: func(notice invalidation.Notice) { notices = append(notices, notice) },
	})
	baseURL, _ := provider.NewBaseURL("https://example.test")
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", BaseURL: provider.Set(baseURL)}); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.Models {
		t.Fatalf("notices = %+v, want models", notices)
	}
}

func TestFailedProviderUpdateDoesNotPublishInvalidation(t *testing.T) {
	var notices []invalidation.Notice
	c := New(Config{
		Providers: &testProviderRegistry{updateErr: errors.New("store unavailable")},
		Catalog: testCatalog{metadata: []ProviderMetadata{providerMetadataFixture(
			t, "compat", ProviderEndpointRequired, ProviderModelsEndpoint, NoEmbeddingCapability(),
		)}},
		Invalidations: func(notice invalidation.Notice) { notices = append(notices, notice) },
	})
	baseURL, _ := provider.NewBaseURL("https://example.test")
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", BaseURL: provider.Set(baseURL)}); err == nil {
		t.Fatal("UpdateProvider unexpectedly succeeded")
	}
	if len(notices) != 0 {
		t.Fatalf("failed update published %+v", notices)
	}
}
