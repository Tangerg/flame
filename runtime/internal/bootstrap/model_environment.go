package bootstrap

import (
	"context"
	"fmt"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"

	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
)

type modelEnvironment struct {
	chatResolver      *modeladapter.ObservedResolver
	utilityRole       *models.RoleSetting
	utilityModel      modeladapter.AuxiliaryResolver
	embeddingRole     *models.RoleSetting
	embeddingResolver *modeladapter.EmbeddingResolver
	liveEmbedder      *modeladapter.RoleEmbedder
	callbacks         *pluginmcp.Callbacks
	capabilities      *modeladapter.Capabilities
}

func buildModelEnvironment(ctx context.Context, cfg assemblyConfig, callbacks *pluginmcp.Callbacks, providers modelProviderAdapters) (modelEnvironment, error) {
	resolver, err := modeladapter.NewChatResolver(cfg.providerRegistry, providers)
	if err != nil {
		return modelEnvironment{}, err
	}
	chatResolver, err := modeladapter.NewObservedResolver(resolver, cfg.stores.ModelInvocations)
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityRole, err := models.LoadRoleSetting(ctx, cfg.stores.UtilityRole)
	if err != nil {
		return modelEnvironment{}, fmt.Errorf("runtime: load utility role: %w", err)
	}

	embeddingRole, err := models.LoadRoleSetting(ctx, cfg.stores.EmbeddingRole)
	if err != nil {
		return modelEnvironment{}, fmt.Errorf("runtime: load embedding role: %w", err)
	}
	embeddingResolver, err := modeladapter.NewEmbeddingResolver(cfg.providerRegistry, providers, cfg.stores.ModelInvocations)
	if err != nil {
		return modelEnvironment{}, err
	}
	liveEmbedder, err := modeladapter.NewRoleEmbedder(embeddingResolver, embeddingRole)
	if err != nil {
		return modelEnvironment{}, err
	}

	capabilities, err := modeladapter.NewCapabilities(providers)
	if err != nil {
		return modelEnvironment{}, err
	}
	environment := modelEnvironment{
		chatResolver:      chatResolver,
		utilityRole:       utilityRole,
		embeddingRole:     embeddingRole,
		embeddingResolver: embeddingResolver,
		capabilities:      capabilities,
	}
	environment.liveEmbedder = liveEmbedder
	environment.callbacks = callbacks
	return environment, nil
}
