package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/provider"
)

func TestResolveProviderConfigAllowsOptionalAPIKey(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	settings, err := resolveProviderConfig(config.Settings{Provider: "ollama", Model: "local-model"})
	if err != nil {
		t.Fatal(err)
	}
	if settings.APIKey.Present() || settings.Provider != "ollama" || settings.Model != "local-model" {
		t.Fatalf("settings = %+v", settings)
	}
	if _, err := resolveProviderConfig(config.Settings{Provider: "openai", Model: "gpt-5.6-sol"}); err == nil {
		t.Fatal("required API key provider was accepted without a key")
	}
}

func TestMCPServersProjectsConfig(t *testing.T) {
	got, err := MCPServers([]config.MCPServer{{
		Name:          "fs",
		Transport:     config.MCPTransportStreamableHTTP,
		Endpoint:      "https://mcp.example",
		Authorization: "Bearer token",
	}})
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	want := mcpserver.Server{
		Name:          testMCPServerName("fs"),
		Transport:     mcpserver.TransportStreamableHTTP,
		Enabled:       true,
		URL:           "https://mcp.example",
		Authorization: "Bearer token",
	}
	if got[0].Name != want.Name ||
		got[0].Transport != want.Transport ||
		!got[0].Enabled ||
		got[0].URL != want.URL ||
		got[0].Authorization != want.Authorization ||
		got[0].Command != want.Command ||
		len(got[0].Args) != 0 {
		t.Fatalf("server = %+v, want %+v", got[0], want)
	}
}

func TestMCPServersRejectsInvalidTransport(t *testing.T) {
	_, err := MCPServers([]config.MCPServer{{
		Name: "unknown", Transport: "websocket", Endpoint: "wss://mcp.example",
	}})
	if err == nil {
		t.Fatal("MCPServers error = nil, want invalid transport")
	}
}

func TestSeedConfiguredProvider(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		stored      map[string]provider.Provider
		cfg         config.Settings
		wantKey     string
		wantBaseURL string
	}{
		{
			name:    "new provider is configured",
			stored:  map[string]provider.Provider{},
			cfg:     config.Settings{Provider: "anthropic", APIKey: config.FileAPIKey("sk-new"), BaseURL: "https://api"},
			wantKey: "sk-new", wantBaseURL: "https://api",
		},
		{
			name: "enabled provider wins over config",
			stored: map[string]provider.Provider{
				"anthropic": bootstrapProvider(t, "anthropic", "sk-stored", "https://stored"),
			},
			cfg:     config.Settings{Provider: "anthropic", APIKey: config.FileAPIKey("sk-new"), BaseURL: "https://api"},
			wantKey: "sk-stored", wantBaseURL: "https://stored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &providerRegistry{stored: tt.stored}
			if err := SeedConfiguredProvider(ctx, reg, tt.cfg); err != nil {
				t.Fatal(err)
			}
			got, ok, err := reg.Get(ctx, tt.cfg.Provider)
			if err != nil || !ok {
				t.Fatalf("Get: ok=%v err=%v", ok, err)
			}
			assertBootstrapProvider(t, got, tt.wantKey, tt.wantBaseURL)
		})
	}
}

func TestSeedConfiguredProviderDoesNotUndoAnExplicitDurableClear(t *testing.T) {
	stored := bootstrapProvider(t, "anthropic", "", "https://stored.example.test")
	registry := &providerRegistry{stored: map[string]provider.Provider{"anthropic": stored}}
	settings := config.Settings{
		Provider: "anthropic",
		APIKey:   config.FileAPIKey("sk-file"),
		BaseURL:  "https://config.example.test",
	}

	if err := SeedConfiguredProvider(t.Context(), registry, settings); err != nil {
		t.Fatal(err)
	}
	effective := registry.stored[settings.Provider]
	if _, configured := effective.Credential(); configured {
		t.Fatal("first-run seed re-enabled an explicitly cleared provider")
	}
	baseURL, present := effective.BaseURL()
	if !present || baseURL.String() != "https://stored.example.test" {
		t.Fatalf("first-run seed replaced durable endpoint = (%q, %t)", baseURL.String(), present)
	}
}

func TestSeedConfiguredProviderDoesNotManufactureOptionalCredential(t *testing.T) {
	registry := &providerRegistry{stored: map[string]provider.Provider{}}
	if err := SeedConfiguredProvider(t.Context(), registry, config.Settings{Provider: "ollama"}); err != nil {
		t.Fatal(err)
	}
	if len(registry.stored) != 0 {
		t.Fatalf("optional provider seed wrote %+v", registry.stored)
	}

	settings := config.Settings{Provider: "ollama", BaseURL: "http://127.0.0.1:22434"}
	if err := SeedConfiguredProvider(t.Context(), registry, settings); err != nil {
		t.Fatal(err)
	}
	stored, found := registry.stored["ollama"]
	if !found {
		t.Fatal("explicit endpoint was not persisted")
	}
	if _, configured := stored.Credential(); configured {
		t.Fatal("optional provider seed invented a credential")
	}
}

func TestSeedConfiguredProviderKeepsEnvironmentKeyOutOfStorageButPersistsEndpoint(t *testing.T) {
	t.Setenv("OPENAI_COMPATIBLE_API_KEY", "sk-env")
	inner := &providerRegistry{stored: map[string]provider.Provider{}}
	cfg := config.Settings{
		Provider: "openai-compatible",
		APIKey:   config.EnvironmentAPIKey("sk-env"),
		BaseURL:  "https://gateway.example.test",
	}

	if err := SeedConfiguredProvider(t.Context(), inner, cfg); err != nil {
		t.Fatal(err)
	}
	registry, err := ProviderRegistry(inner, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stored := inner.stored[cfg.Provider]
	if _, configured := stored.Credential(); configured {
		t.Fatal("environment credential was persisted")
	}
	storedBaseURL, present := stored.BaseURL()
	if !present || storedBaseURL.String() != cfg.BaseURL {
		t.Fatalf("stored base URL = (%q, %v), want %q", storedBaseURL.String(), present, cfg.BaseURL)
	}
	effective, ok, err := registry.Get(t.Context(), cfg.Provider)
	if err != nil || !ok {
		t.Fatalf("effective provider ok=%v, err=%v", ok, err)
	}
	effectiveCredential, _ := effective.Credential()
	effectiveKey, _ := effectiveCredential.APIKey()
	effectiveSource, _ := effectiveCredential.Source()
	wantKey, _ := cfg.APIKey.EnvironmentValue()
	if effectiveSource != provider.KeyEnvironment || effectiveKey.Reveal() != wantKey {
		t.Fatalf("effective credential = (%q, %q)", effectiveKey.Reveal(), effectiveSource)
	}
}

func TestGenericEnvironmentKeyNeverCrossesTheDurableProviderBoundary(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(directory, "config.yaml"),
		[]byte("provider: anthropic\napiKey: sk-file\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_APIKEY", "sk-process")
	t.Setenv("ANTHROPIC_API_KEY", "")

	settings, err := LoadConfig([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	inner := &providerRegistry{stored: map[string]provider.Provider{}}
	if err := SeedConfiguredProvider(t.Context(), inner, settings); err != nil {
		t.Fatal(err)
	}
	registry, err := ProviderRegistry(inner, settings)
	if err != nil {
		t.Fatal(err)
	}

	if stored, found := inner.stored[settings.Provider]; found {
		if _, configured := stored.Credential(); configured {
			t.Fatal("generic environment credential was persisted")
		}
	}
	effective, found, err := registry.Get(t.Context(), settings.Provider)
	if err != nil || !found {
		t.Fatalf("effective provider found=%t, err=%v", found, err)
	}
	credential, configured := effective.Credential()
	source, hasSource := credential.Source()
	if !configured || !hasSource || source != provider.KeyEnvironment {
		t.Fatalf("effective credential configured=%t, source=(%q, %t)", configured, source, hasSource)
	}
}

type providerRegistry struct {
	stored map[string]provider.Provider
}

func (p *providerRegistry) List(context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(p.stored))
	for _, stored := range p.stored {
		out = append(out, stored)
	}
	return out, nil
}

func (p *providerRegistry) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	stored, ok := p.stored[id]
	return stored, ok, nil
}

func (p *providerRegistry) Update(_ context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	stored, found := p.stored[id]
	if !found {
		var err error
		stored, err = provider.New(id)
		if err != nil {
			return provider.Provider{}, err
		}
	}
	stored, err := stored.Apply(patch)
	if err != nil {
		return provider.Provider{}, err
	}
	p.stored[id] = stored
	return stored, nil
}

func bootstrapProvider(t *testing.T, id, rawKey, rawBaseURL string) provider.Provider {
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

func assertBootstrapProvider(t *testing.T, entry provider.Provider, wantKey, wantBaseURL string) {
	t.Helper()
	key, configured := entry.APIKey()
	if !configured || key.Reveal() != wantKey {
		t.Fatalf("credential = (%q, %v), want %q", key.Reveal(), configured, wantKey)
	}
	baseURL, present := entry.BaseURL()
	if !present || baseURL.String() != wantBaseURL {
		t.Fatalf("base URL = (%q, %v), want %q", baseURL.String(), present, wantBaseURL)
	}
}
