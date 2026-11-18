// Package bootstrap is the composition root: it adapts process config and
// environment into runtime construction inputs, wires the rings, and owns host
// lifecycle.
package bootstrap

import (
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
)

// providerRegistry wraps the durable provider registry with env-key fallback.
func providerRegistry(registry models.ProviderRegistry, settings config.Settings) (models.ProviderRegistry, error) {
	environmentKeys := make(map[string]string)
	if apiKey, present := settings.APIKey.EnvironmentValue(); present {
		environmentKeys[settings.DefaultSelection.Provider()] = apiKey
	}
	return modeladapter.WithEnvironmentKeys(registry, environmentKeys)
}
