package promptsource

import (
	"context"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

// Recipes lists project recipes layered over one configured global
// directory.
type Recipes struct{ userDir string }

// NewRecipes returns the workspace discovery adapter for recipes.
func NewRecipes(userDir string) Recipes {
	return Recipes{userDir: userDir}
}

var _ workspaceapp.RecipeLister = Recipes{}

func (w Recipes) List(ctx context.Context, cwd string) ([]workspaceapp.Recipe, error) {
	return listRecipes(ctx, recipeDir(cwd), w.userDir)
}

// Skills lists project Skills layered over one configured user
// directory.
type Skills struct{ userDir string }

// NewSkills returns the workspace Skill-discovery adapter.
func NewSkills(userDir string) Skills {
	return Skills{userDir: userDir}
}

var _ workspaceapp.SkillCatalog = Skills{}

func (w Skills) List(ctx context.Context, cwd string) ([]workspaceapp.SkillSummary, error) {
	return ListSkills(ctx, cwd, w.userDir)
}
