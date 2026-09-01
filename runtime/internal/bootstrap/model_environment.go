package bootstrap

import (
	"context"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	agentmemoryapp "github.com/Tangerg/flame/runtime/internal/application/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// modelEnvironment is the composition-time graph shared by interactive model
// execution, utility-model work, and embedding-backed search. Its live role
// states let configuration changes take effect without rebuilding the Host.
type modelEnvironment struct {
	chatResolver       ChatResolver
	utilityRoleState   *models.RoleState
	utilityClient      modeladapter.AuxiliaryResolver
	embeddingRoleState *models.RoleState
	embeddingResolver  *modeladapter.EmbeddingResolver
	liveEmbedder       *modeladapter.RoleEmbedder
	agentMemorySearch  *agentmemoryapp.Searcher
}

func buildModelEnvironment(ctx context.Context, cfg Config, defaultSelection modelref.Selection) (modelEnvironment, error) {
	chatResolver := cfg.ChatResolver
	utilityRole, err := loadUtilityRole(ctx, cfg.UtilityRoleStore)
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityRoleState := models.NewRoleState(utilityRole)

	embeddingRole, err := loadEmbeddingRole(ctx, cfg.EmbeddingRoleStore)
	if err != nil {
		return modelEnvironment{}, err
	}
	embeddingRoleState := models.NewRoleState(embeddingRole)
	embeddingResolver := modeladapter.NewEmbeddingResolver(cfg.ProviderRegistry)
	liveEmbedder := modeladapter.NewRoleEmbedder(embeddingResolver, embeddingRoleState)

	environment := modelEnvironment{
		chatResolver:       chatResolver,
		utilityRoleState:   utilityRoleState,
		utilityClient:      modeladapter.LiveUtilityClient(chatResolver, defaultSelection, utilityRoleState),
		embeddingRoleState: embeddingRoleState,
		embeddingResolver:  embeddingResolver,
		liveEmbedder:       liveEmbedder,
	}
	if cfg.AgentMemoryStore != nil {
		environment.agentMemorySearch = agentmemoryapp.NewSearcher(cfg.AgentMemoryStore, liveEmbedder.ResolveMemory)
	}
	return environment, nil
}
