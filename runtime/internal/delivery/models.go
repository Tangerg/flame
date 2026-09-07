package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	ModelsList             Name = "models.list"
	ModelsGetUtilityRole   Name = "models.getUtilityRole"
	ModelsSetUtilityRole   Name = "models.setUtilityRole"
	ModelsGetEmbeddingRole Name = "models.getEmbeddingRole"
	ModelsSetEmbeddingRole Name = "models.setEmbeddingRole"
)

func registerModels(registry *Registry) {
	registry.Query(MethodMeta{Name: ModelsList},
		(*Handler).ListModels)

	registry.Query(MethodMeta{Name: ModelsGetUtilityRole},
		func(service *Handler, ctx context.Context, _ struct{}) (*protocol.UtilityRole, error) {
			return service.GetUtilityRole(ctx)
		})

	registry.Command(MethodMeta{Name: ModelsSetUtilityRole},
		(*Handler).SetUtilityRole)

	registry.Query(MethodMeta{Name: ModelsGetEmbeddingRole},
		func(service *Handler, ctx context.Context, _ struct{}) (*protocol.EmbeddingRole, error) {
			return service.GetEmbeddingRole(ctx)
		})

	registry.Command(MethodMeta{Name: ModelsSetEmbeddingRole},
		(*Handler).SetEmbeddingRole)
}
