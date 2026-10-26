package promptsource

import (
	"context"
	"errors"
	"fmt"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	sdk "github.com/Tangerg/scope/skills"
)

// Skills lists project Skills layered over one configured user
// directory.
type Skills struct {
	userDir  string
	packages PackageSkills
}

// NewSkills returns the workspace Skill-discovery adapter.
func NewSkills(userDir string, packages PackageSkills) Skills {
	return Skills{userDir: userDir, packages: packages}
}

var _ workspaceapp.SkillCatalog = Skills{}

func (w Skills) List(ctx context.Context, cwd string) (workspaceapp.SkillDiscovery, error) {
	return ListSkills(ctx, cwd, w.userDir, w.packages)
}

func (w Skills) Get(ctx context.Context, cwd, name string) (workspaceapp.SkillDetail, error) {
	if err := domainskills.ValidateName(name); err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	selected, err := resolveSkills(ctx, cwd, w.userDir, w.packages)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	detail, err := selected.get(ctx, name)
	if cancelErr := skillSourceContextError(ctx, "inspect"); cancelErr != nil {
		return workspaceapp.SkillDetail{}, errors.Join(err, cancelErr)
	}
	if errors.Is(err, sdk.ErrSkillNotFound) {
		return workspaceapp.SkillDetail{}, fmt.Errorf("%w: %w", domainskills.ErrNotFound, err)
	}
	if errors.Is(err, sdk.ErrInvalidSkill) || errors.Is(err, domainskills.ErrDocumentTooLarge) {
		return workspaceapp.SkillDetail{}, fmt.Errorf("%w: %w", workspaceapp.ErrSkillUnavailable, err)
	}
	return detail, err
}
