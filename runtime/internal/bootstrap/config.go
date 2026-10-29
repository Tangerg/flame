// Package bootstrap is the composition root: it adapts process config and
// environment into runtime construction inputs, wires the rings, and owns host
// lifecycle.
package bootstrap

import (
	"fmt"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
)

// LoadConfig loads process settings and the explicit default selection.
func loadConfig(configDirectories []string) (config.Settings, error) {
	cfg, err := config.Load(configDirectories)
	if err != nil {
		return config.Settings{}, err
	}
	return resolveProviderConfig(cfg)
}

func resolveProviderConfig(settings config.Settings) (config.Settings, error) {
	if settings.Model == "" {
		if settings.Provider != "anthropic" {
			return config.Settings{}, fmt.Errorf("config: provider %q requires an explicit model", settings.Provider)
		}
		settings.Model = "claude-opus-5"
	}
	if _, err := modelchoice.New(settings.Provider, settings.Model); err != nil {
		return config.Settings{}, fmt.Errorf("config: model selection: %w", err)
	}
	return settings, nil
}

// ProviderRegistry wraps the durable provider registry with env-key fallback.
func providerRegistry(registry models.ProviderRegistry, settings config.Settings) (models.ProviderRegistry, error) {
	environmentKeys := make(map[string]string)
	if apiKey, present := settings.APIKey.EnvironmentValue(); present {
		environmentKeys[settings.Provider] = apiKey
	}
	return modeladapter.WithEnvironmentKeys(registry, environmentKeys)
}
