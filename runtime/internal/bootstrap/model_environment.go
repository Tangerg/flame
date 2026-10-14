package bootstrap

import (
	"context"

	pluginsdk "github.com/Tangerg/flame/runtime/plugin"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// modelEnvironment is the composition-time graph shared by interactive model
// execution, utility-model work, and embedding-backed search. Its live role
// states let configuration changes take effect without rebuilding the Instance.
type modelEnvironment struct {
	defaultSelection   modelref.Selection
	chatResolver       ChatResolver
	utilityRoleState   *models.RoleState
	utilityModel       modeladapter.AuxiliaryResolver
	embeddingRoleState *models.RoleState
	embeddingResolver  *modeladapter.EmbeddingResolver
	liveEmbedder       *modeladapter.RoleEmbedder
	callbacks          *pluginsdk.Callbacks
}

func buildModelEnvironment(ctx context.Context, cfg Config, defaultSelection modelref.Selection) (modelEnvironment, error) {
	chatResolver, err := modeladapter.NewObservedResolver(cfg.ChatResolver, cfg.Stores.ModelInvocations, cfg.Pricing)
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityRole, err := loadUtilityRole(ctx, cfg.Stores.UtilityRole)
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityRoleState := models.NewRoleState(utilityRole)

	embeddingRole, err := loadEmbeddingRole(ctx, cfg.Stores.EmbeddingRole)
	if err != nil {
		return modelEnvironment{}, err
	}
	embeddingRoleState := models.NewRoleState(embeddingRole)
	embeddingResolver, err := modeladapter.NewEmbeddingResolver(cfg.ProviderRegistry)
	if err != nil {
		return modelEnvironment{}, err
	}
	liveEmbedder, err := modeladapter.NewRoleEmbedder(embeddingResolver, embeddingRoleState)
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityModel, err := modeladapter.LiveUtilityModel(chatResolver, defaultSelection, utilityRoleState)
	if err != nil {
		return modelEnvironment{}, err
	}

	environment := modelEnvironment{
		defaultSelection:   defaultSelection,
		chatResolver:       chatResolver,
		utilityRoleState:   utilityRoleState,
		utilityModel:       utilityModel,
		embeddingRoleState: embeddingRoleState,
		embeddingResolver:  embeddingResolver,
	}
	environment.liveEmbedder = liveEmbedder
	environment.callbacks = pluginsdk.NewCallbacks()
	return environment, nil
}
