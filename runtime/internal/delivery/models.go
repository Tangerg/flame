package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	ModelsList             Name = "models.list"
	ModelsGetUtilityRole   Name = "models.getUtilityRole"
	ModelsSetUtilityRole   Name = "models.setUtilityRole"
	ModelsGetEmbeddingRole Name = "models.getEmbeddingRole"
	ModelsSetEmbeddingRole Name = "models.setEmbeddingRole"
)

func registerModels(registry *Registry) {
	registry.query(MethodMeta{Name: ModelsList, GoName: "ListModels", Errors: []string{protocol.ErrProviderError.Error()}},
		func(service interface {
			ListModels(context.Context, protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error)
		}, ctx context.Context, request protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
			return service.ListModels(ctx, request)
		})

	registry.query(MethodMeta{Name: ModelsGetUtilityRole, GoName: "GetUtilityRole"},
		func(service interface {
			GetUtilityRole(context.Context) (*protocol.UtilityRole, error)
		}, ctx context.Context, _ struct{}) (*protocol.UtilityRole, error) {
			return service.GetUtilityRole(ctx)
		})

	registry.command(MethodMeta{Name: ModelsSetUtilityRole, GoName: "SetUtilityRole"},
		func(service interface {
			SetUtilityRole(context.Context, protocol.UtilityRole) (*protocol.UtilityRole, error)
		}, ctx context.Context, request protocol.UtilityRole) (*protocol.UtilityRole, error) {
			return service.SetUtilityRole(ctx, request)
		})

	registry.query(MethodMeta{Name: ModelsGetEmbeddingRole, GoName: "GetEmbeddingRole"},
		func(service interface {
			GetEmbeddingRole(context.Context) (*protocol.EmbeddingRole, error)
		}, ctx context.Context, _ struct{}) (*protocol.EmbeddingRole, error) {
			return service.GetEmbeddingRole(ctx)
		})

	registry.command(MethodMeta{Name: ModelsSetEmbeddingRole, GoName: "SetEmbeddingRole"},
		func(service interface {
			SetEmbeddingRole(context.Context, protocol.EmbeddingRole) (*protocol.EmbeddingRole, error)
		}, ctx context.Context, request protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
			return service.SetEmbeddingRole(ctx, request)
		})
}
