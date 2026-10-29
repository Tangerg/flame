package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	modelsList             Name = "models.list"
	modelsGetUtilityRole   Name = "models.getUtilityRole"
	modelsSetUtilityRole   Name = "models.setUtilityRole"
	modelsGetEmbeddingRole Name = "models.getEmbeddingRole"
	ModelsSetEmbeddingRole Name = "models.setEmbeddingRole"
)

func registerModels(registry *Registry) {
	registry.query(MethodMeta{Name: modelsList, GoName: "ListModels", Errors: modelProviderErrors(protocol.ErrProviderError.Error())},
		func(service interface {
			ListModels(context.Context, protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error)
		}, ctx context.Context, request protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
			return service.ListModels(ctx, request)
		})

	registry.query(MethodMeta{Name: modelsGetUtilityRole, GoName: "GetUtilityRole"},
		func(service interface {
			GetUtilityRole(context.Context) (*protocol.UtilityRole, error)
		}, ctx context.Context, _ struct{}) (*protocol.UtilityRole, error) {
			return service.GetUtilityRole(ctx)
		})

	registry.command(MethodMeta{Name: modelsSetUtilityRole, GoName: "SetUtilityRole", Errors: modelProviderErrors()},
		func(service interface {
			SetUtilityRole(context.Context, protocol.UtilityRole) (*protocol.UtilityRole, error)
		}, ctx context.Context, request protocol.UtilityRole) (*protocol.UtilityRole, error) {
			return service.SetUtilityRole(ctx, request)
		})

	registry.query(MethodMeta{Name: modelsGetEmbeddingRole, GoName: "GetEmbeddingRole"},
		func(service interface {
			GetEmbeddingRole(context.Context) (*protocol.EmbeddingRole, error)
		}, ctx context.Context, _ struct{}) (*protocol.EmbeddingRole, error) {
			return service.GetEmbeddingRole(ctx)
		})

	registry.command(MethodMeta{Name: ModelsSetEmbeddingRole, GoName: "SetEmbeddingRole", Errors: modelProviderErrors()},
		func(service interface {
			SetEmbeddingRole(context.Context, protocol.EmbeddingRole) (*protocol.EmbeddingRole, error)
		}, ctx context.Context, request protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
			return service.SetEmbeddingRole(ctx, request)
		})
}

func modelProviderErrors(other ...string) []string {
	return append([]string{protocol.ErrPluginUnavailable.Error(), protocol.ErrPluginStale.Error()}, other...)
}
