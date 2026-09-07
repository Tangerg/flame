package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const RecipesList Name = "recipes.list"

func registerRecipes(registry *Registry) {
	registry.Query(MethodMeta{
		Name:   RecipesList,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, (*Handler).ListRecipes)
}
