package bootstrap

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/config"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// SeedConfiguredProvider writes config-file provider values only when the
// durable registry has no row for that provider. Any existing row represents
// an explicit runtime-owned value, including cleared credential/endpoint
// fields, and wins as a whole. Callers must pass the unoverlaid durable
// registry so an environment credential cannot masquerade as stored state.
func SeedConfiguredProvider(ctx context.Context, registry models.ProviderRegistry, cfg config.Settings) error {
	id := cfg.Provider
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

// SeedUtilityRole writes the config-file utility model into the store on first
// run (when no row exists yet), pinned to the default provider. A role already
// persisted via models.setUtilityRole is left untouched — runtime edits win
// over the config file. An empty / identical-to-main UtilityModel seeds
// nothing (maintenance then runs on the main model).
func SeedUtilityRole(ctx context.Context, store UtilityRoleStore, cfg config.Settings) error {
	_, present, err := store.LoadUtilityRole(ctx)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	if cfg.UtilityModel == "" || cfg.UtilityModel == cfg.Model {
		return nil
	}
	role, err := modelref.NewRole(cfg.Provider, cfg.UtilityModel)
	if err != nil {
		return err
	}
	return store.SaveUtilityRole(ctx, role)
}
