package bootstrap

import (
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
)

func providerRegistry(registry models.ProviderRegistry, settings config.Settings) (models.ProviderRegistry, error) {
	environmentKeys := make(map[string]string)
	if apiKey, present := settings.APIKey.EnvironmentValue(); present {
		environmentKeys[settings.DefaultSelection.Provider()] = apiKey
	}
	return modeladapter.WithEnvironmentKeys(registry, environmentKeys)
}
