package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const RecipesList Name = "recipes.list"

func registerRecipes(registry *Registry) {
	registry.query(MethodMeta{
		Name:   RecipesList,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, (*Handler).ListRecipes)
}
