package promptsource

import (
	"context"
	"errors"
	"fmt"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	skillspec "github.com/Tangerg/scope/skills"
)

type Skills struct {
	userDir  string
	packages SkillSources
}

func NewSkills(userDir string, packages SkillSources) (Skills, error) {
	if err := packages.Validate(); err != nil {
		return Skills{}, err
	}
	return Skills{userDir: userDir, packages: packages}, nil
}

var _ workspaceapp.SkillCatalog = Skills{}

func (w Skills) List(ctx context.Context, cwd string) (workspaceapp.SkillDiscovery, error) {
	return ListSkills(ctx, cwd, w.userDir, w.packages)
}

func (w Skills) Get(ctx context.Context, cwd, name string) (workspaceapp.SkillDetail, error) {
	if err := domainskills.ValidateName(name); err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	selected, err := resolveSkills(ctx, cwd, w.userDir, w.packages, nil)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	detail, err := selected.get(ctx, name)
	if cancelErr := skillSourceContextError(ctx, skillSourceInspect); cancelErr != nil {
		return workspaceapp.SkillDetail{}, errors.Join(err, cancelErr)
	}
	if errors.Is(err, skillspec.ErrSkillNotFound) {
		return workspaceapp.SkillDetail{}, fmt.Errorf("%w: %w", domainskills.ErrNotFound, err)
	}
	if errors.Is(err, skillspec.ErrInvalidSkill) || errors.Is(err, domainskills.ErrDocumentTooLarge) {
		return workspaceapp.SkillDetail{}, fmt.Errorf("%w: %w", workspaceapp.ErrSkillUnavailable, err)
	}
	return detail, err
}
