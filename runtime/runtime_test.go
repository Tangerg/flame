package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestFreshRuntimeSubscriptionObservesFirstSkill(t *testing.T) {
	for _, name := range []string{
		"FLAME_PROVIDER", "FLAME_MODEL", "FLAME_APIKEY", "FLAME_BASEURL",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_MODEL", "claude-opus-5")
	data := testsupport.TempDirectory(t)
	rt, err := Open(t.Context(), Config{
		DataDirectory: data, UserHomePath: testsupport.TempDirectory(t), DefaultWorkspacePath: testsupport.TempDirectory(t),
		ConfigDirectories: []string{testsupport.TempDirectory(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, events, err := rt.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged, protocol.TopicSessionsChanged},
	}, protocol.SubscriptionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(data, "skills", "first", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("---\nname: first\ndescription: First skill\n---\nUse the first skill.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for event, err := range events {
		if err != nil {
			t.Fatal(err)
		}
		if event.Type == protocol.RuntimeSkillsChanged {
			return
		}
	}
	t.Fatal("subscription ended before the first skill became observable")
}

func TestRuntimeRestartDoesNotUndoAStoredProviderClear(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "")
	t.Setenv("FLAME_APIKEY", "")
	t.Setenv("FLAME_BASEURL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	configDirectory := testsupport.TempDirectory(t)
	if err := os.WriteFile(
		filepath.Join(configDirectory, "config.yaml"),
		[]byte("provider: anthropic\nmodel: claude-opus-5\napiKey: sk-file\nbaseUrl: https://config.example.test\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	config := Config{
		DataDirectory:        testsupport.TempDirectory(t),
		DefaultWorkspacePath: testsupport.TempDirectory(t),
		UserHomePath:         testsupport.TempDirectory(t),
		ConfigDirectories:    []string{configDirectory},
	}

	runtime, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.UpdateProvider(t.Context(), protocol.UpdateProviderRequest{
		Provider: "anthropic",
		APIKey:   &protocol.ProviderConfigChange{Type: protocol.ProviderConfigClear},
	}, protocol.CommandOptions{IdempotencyKey: "clear-provider-before-restart"}); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	providers, err := reopened.ListProviders(t.Context(), protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range providers.Data {
		if candidate.ID != "anthropic" {
			continue
		}
		if candidate.Credential != nil || candidate.Configured {
			t.Fatalf("restart re-enabled cleared provider = %+v", candidate)
		}
		if candidate.BaseURL == nil || *candidate.BaseURL != "https://config.example.test" {
			t.Fatalf("restart changed stored provider endpoint = %+v", candidate)
		}
		return
	}
	t.Fatal("anthropic provider missing after restart")
}

func TestRuntimeCanConfigureRequiredKeyProviderAfterStartup(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "")
	t.Setenv("FLAME_MODEL", "")
	t.Setenv("FLAME_APIKEY", "")
	t.Setenv("FLAME_BASEURL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	configDirectory := testsupport.TempDirectory(t)
	if err := os.WriteFile(
		filepath.Join(configDirectory, "config.yaml"),
		[]byte("provider: anthropic\nmodel: claude-opus-5\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	config := Config{
		DataDirectory:        testsupport.TempDirectory(t),
		DefaultWorkspacePath: testsupport.TempDirectory(t),
		UserHomePath:         testsupport.TempDirectory(t),
		ConfigDirectories:    []string{configDirectory},
	}

	runtime, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := runtime.ListProviders(t.Context(), protocol.CallOptions{})
	if err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	configured := providerByID(t, providers.Data, "anthropic")
	if configured.Configured || configured.Credential != nil {
		_ = runtime.Close()
		t.Fatalf("provider started configured without a credential = %+v", configured)
	}

	key := "sk-stored-after-startup"
	configured, err = runtime.UpdateProvider(t.Context(), protocol.UpdateProviderRequest{
		Provider: "anthropic",
		APIKey: &protocol.ProviderConfigChange{
			Type:  protocol.ProviderConfigSet,
			Value: &key,
		},
	}, protocol.CommandOptions{IdempotencyKey: "configure-provider-after-startup"})
	if err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if !configured.Configured || configured.Credential == nil || configured.Credential.Source != protocol.ProviderKeySourceStored {
		_ = runtime.Close()
		t.Fatalf("provider update did not establish stored configuration = %+v", configured)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	providers, err = reopened.ListProviders(t.Context(), protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	configured = providerByID(t, providers.Data, "anthropic")
	if !configured.Configured || configured.Credential == nil || configured.Credential.Source != protocol.ProviderKeySourceStored {
		t.Fatalf("stored provider configuration did not survive restart = %+v", configured)
	}
}

func providerByID(t *testing.T, providers []protocol.Provider, id string) *protocol.Provider {
	t.Helper()
	for index := range providers {
		if providers[index].ID == id {
			return &providers[index]
		}
	}
	t.Fatalf("provider %q is missing", id)
	return nil
}
