// Package bootstrap is the composition root: it adapts process config and
// environment into runtime construction inputs, wires the rings, and owns host
// lifecycle.
package bootstrap

import (
	"fmt"
	"os"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/llm"
)

// LoadConfig loads the app config and resolves provider defaults plus env-key
// overrides used by the runtime process.
func LoadConfig(configDirectories []string) (config.Settings, error) {
	cfg, err := config.Load(configDirectories)
	if err != nil {
		return config.Settings{}, err
	}
	return resolveProviderConfig(cfg)
}

func resolveProviderConfig(settings config.Settings) (config.Settings, error) {
	profile, found := llm.LookupProvider(llm.Provider(settings.Provider))
	if !found {
		return config.Settings{}, fmt.Errorf("config: unknown provider %q (see providers.list for the supported set)", settings.Provider)
	}
	if settings.Model == "" {
		defaultModel, hasDefault := profile.DefaultChatModel()
		if !hasDefault {
			return config.Settings{}, fmt.Errorf("config: provider %q requires an explicit model", settings.Provider)
		}
		settings.Model = defaultModel
	}
	apiKeyEnvironmentVariable := profile.CredentialEnvironment()
	if apiKey := os.Getenv(apiKeyEnvironmentVariable); apiKey != "" {
		settings.APIKey = config.EnvironmentAPIKey(apiKey)
	}
	return settings, nil
}

// ProviderRegistry wraps the durable provider registry with env-key fallback.
func ProviderRegistry(registry models.ProviderRegistry, settings config.Settings) (models.ProviderRegistry, error) {
	environmentKeys := llm.EnvKeys()
	if apiKey, present := settings.APIKey.EnvironmentValue(); present {
		environmentKeys[settings.Provider] = apiKey
	}
	return modeladapter.WithEnvironmentKeys(registry, environmentKeys)
}
