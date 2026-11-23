package bootstrap

import (
	"context"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"

	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
)

type modelEnvironment struct {
	chatResolver       *modeladapter.ObservedResolver
	utilityRoleState   *models.RoleState
	utilityModel       modeladapter.AuxiliaryResolver
	embeddingRoleState *models.RoleState
	embeddingResolver  *modeladapter.EmbeddingResolver
	liveEmbedder       *modeladapter.RoleEmbedder
	callbacks          *pluginmcp.Callbacks
	capabilities       *modeladapter.Capabilities
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
	utilityRole, err := loadRole(ctx, cfg.stores.UtilityRole, "utility")
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityRoleState := models.NewRoleState(utilityRole)

	embeddingRole, err := loadRole(ctx, cfg.stores.EmbeddingRole, "embedding")
	if err != nil {
		return modelEnvironment{}, err
	}
	embeddingRoleState := models.NewRoleState(embeddingRole)
	embeddingResolver, err := modeladapter.NewEmbeddingResolver(cfg.providerRegistry, providers, cfg.stores.ModelInvocations)
	if err != nil {
		return modelEnvironment{}, err
	}
	liveEmbedder, err := modeladapter.NewRoleEmbedder(embeddingResolver, embeddingRoleState)
	if err != nil {
		return modelEnvironment{}, err
	}

	capabilities, err := modeladapter.NewCapabilities(providers)
	if err != nil {
		return modelEnvironment{}, err
	}
	environment := modelEnvironment{
		chatResolver:       chatResolver,
		utilityRoleState:   utilityRoleState,
		embeddingRoleState: embeddingRoleState,
		embeddingResolver:  embeddingResolver,
		capabilities:       capabilities,
	}
	environment.liveEmbedder = liveEmbedder
	environment.callbacks = callbacks
	return environment, nil
}
