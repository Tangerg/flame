package bootstrap

import (
	"context"
	"fmt"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
)

type modelEnvironment struct {
	chatResolver      *modeladapter.ObservedResolver
	utilityModel      modeladapter.AuxiliaryResolver
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
	if _, _, err := cfg.stores.UtilityRole.Load(ctx); err != nil {
		return modelEnvironment{}, fmt.Errorf("runtime: load utility role: %w", err)
	}
	if _, _, err := cfg.stores.EmbeddingRole.Load(ctx); err != nil {
		return modelEnvironment{}, fmt.Errorf("runtime: load embedding role: %w", err)
	}
	embeddingResolver, err := modeladapter.NewEmbeddingResolver(cfg.providerRegistry, providers, cfg.stores.ModelInvocations)
	if err != nil {
		return modelEnvironment{}, err
	}
	liveEmbedder, err := modeladapter.NewRoleEmbedder(embeddingResolver, cfg.stores.EmbeddingRole)
	if err != nil {
		return modelEnvironment{}, err
	}

	capabilities, err := modeladapter.NewCapabilities(providers)
	if err != nil {
		return modelEnvironment{}, err
	}
	return modelEnvironment{
		chatResolver:      chatResolver,
		embeddingResolver: embeddingResolver,
		liveEmbedder:      liveEmbedder,
		callbacks:         callbacks,
		capabilities:      capabilities,
	}, nil
}
