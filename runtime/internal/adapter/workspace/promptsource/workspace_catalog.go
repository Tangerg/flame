package promptsource

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/Tangerg/scope/skills"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
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
	return listRecipes(ctx, RecipeDirectory(cwd), w.userDir)
}

// Skills lists project Skills layered over one configured user
// directory.
type Skills struct{ userDir string }

// NewSkills returns the workspace Skill-discovery adapter.
func NewSkills(userDir string) Skills {
	return Skills{userDir: userDir}
}

var _ workspaceapp.SkillCatalog = Skills{}

func (w Skills) List(ctx context.Context, cwd string) (workspaceapp.SkillDiscovery, error) {
	return ListSkills(ctx, cwd, w.userDir)
}

func (w Skills) Get(ctx context.Context, cwd, name string) (workspaceapp.SkillDetail, error) {
	if err := sdk.ValidateName(name); err != nil {
		return workspaceapp.SkillDetail{}, fmt.Errorf("%w: %w", workspaceapp.ErrSkillUnavailable, err)
	}
	layers, err := openRuntimeSkillLayers(cwd, w.userDir)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	detail, err := layers.get(ctx, name)
	if context.Cause(ctx) != nil {
		return workspaceapp.SkillDetail{}, context.Cause(ctx)
	}
	if errors.Is(err, sdk.ErrSkillNotFound) || errors.Is(err, sdk.ErrInvalidSkill) || errors.Is(err, domainskills.ErrDocumentTooLarge) {
		return workspaceapp.SkillDetail{}, fmt.Errorf("%w: %w", workspaceapp.ErrSkillUnavailable, err)
	}
	return detail, err
}
