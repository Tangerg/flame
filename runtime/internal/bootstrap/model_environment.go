package bootstrap

import (
	"context"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
)

// modelEnvironment is the composition-time graph shared by interactive model
// execution, utility-model work, and embedding-backed search. Its live role
// states let configuration changes take effect without rebuilding the Instance.
type modelEnvironment struct {
	defaultSelection   modelchoice.Selection
	chatResolver       chatResolver
	utilityRoleState   *models.RoleState
	utilityModel       modeladapter.AuxiliaryResolver
	embeddingRoleState *models.RoleState
	embeddingResolver  *modeladapter.EmbeddingResolver
	liveEmbedder       *modeladapter.RoleEmbedder
	callbacks          *qimumcp.Callbacks
}

func buildModelEnvironment(ctx context.Context, cfg assemblyConfig, defaultSelection modelchoice.Selection) (modelEnvironment, error) {
	chatResolver, err := modeladapter.NewObservedResolver(cfg.chatResolver, cfg.stores.ModelInvocations, cfg.pricing)
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityRole, err := loadUtilityRole(ctx, cfg.stores.UtilityRole)
	if err != nil {
		return modelEnvironment{}, err
	}
	utilityRoleState := models.NewRoleState(utilityRole)

	embeddingRole, err := loadEmbeddingRole(ctx, cfg.stores.EmbeddingRole)
	if err != nil {
		return modelEnvironment{}, err
	}
	embeddingRoleState := models.NewRoleState(embeddingRole)
	embeddingResolver, err := modeladapter.NewEmbeddingResolver(cfg.providerRegistry, cfg.stores.ModelInvocations, cfg.pricing)
	if err != nil {
		return modelEnvironment{}, err
	}
	liveEmbedder, err := modeladapter.NewRoleEmbedder(embeddingResolver, embeddingRoleState)
	if err != nil {
		return modelEnvironment{}, err
	}

	environment := modelEnvironment{
		defaultSelection:   defaultSelection,
		chatResolver:       chatResolver,
		utilityRoleState:   utilityRoleState,
		embeddingRoleState: embeddingRoleState,
		embeddingResolver:  embeddingResolver,
	}
	environment.liveEmbedder = liveEmbedder
	environment.callbacks = qimumcp.NewCallbacks()
	return environment, nil
}
