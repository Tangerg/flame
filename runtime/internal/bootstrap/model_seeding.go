package bootstrap

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// Callers must pass the unoverlaid durable registry so an environment credential
// cannot masquerade as stored state.
func seedConfiguredProvider(ctx context.Context, registry models.ProviderRegistry, cfg config.Settings) error {
	id := cfg.DefaultSelection.Provider()
	_, ok, err := registry.Get(ctx, id)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	patch := provider.Patch{}
	if rawAPIKey, present := cfg.APIKey.FileValue(); present {
		apiKey, keyErr := provider.NewAPIKey(rawAPIKey)
		if keyErr != nil {
			return keyErr
		}
		patch.APIKey = provider.Set(apiKey)
	}
	if cfg.BaseURL != "" {
		baseURL, parseErr := provider.NewBaseURL(cfg.BaseURL)
		if parseErr != nil {
			return parseErr
		}
		patch.BaseURL = provider.Set(baseURL)
	}
	if patch.Empty() {
		return nil
	}
	_, err = registry.Update(ctx, id, patch)
	return err
}

type utilityRoleSeedStore interface {
	roleLoader
	Save(ctx context.Context, role modelref.Role) error
}

func seedUtilityRole(ctx context.Context, store utilityRoleSeedStore, cfg config.Settings) error {
	_, present, err := store.Load(ctx)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	if cfg.UtilityModel == "" || cfg.UtilityModel == cfg.DefaultSelection.Model() {
		return nil
	}
	role, err := modelref.NewRole(cfg.DefaultSelection.Provider(), cfg.UtilityModel)
	if err != nil {
		return err
	}
	return store.Save(ctx, role)
}
