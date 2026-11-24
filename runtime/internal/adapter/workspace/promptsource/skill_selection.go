package promptsource

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	skillspec "github.com/Tangerg/scope/skills"
)

var errPackageSkillConflict = errors.New("promptsource: package Skill name conflict")

type skillDocumentSource interface {
	skillspec.ResourceSource
	document(context.Context, string) (*skillspec.Skill, []byte, error)
	documentPath(string) string
}

type selectedSkill struct {
	source      skillDocumentSource
	origin      workspaceapp.SkillSource
	packageCopy bool
}

type resolvedSkills struct {
	byName map[string]selectedSkill
}

func resolveSkills(ctx context.Context, workspaceRoot, userDir string, sources SkillSources, retained []plugin.Dependency) (*resolvedSkills, error) {
	repository, err := openRuntimeSkillSource(RepositorySkillDir(workspaceRoot), workspaceRoot)
	if err != nil {
		return nil, err
	}
	user, err := openRuntimeSkillSource(userDir, userDir)
	if err != nil {
		return nil, err
	}
	packages := sources.Packages
	bundles, err := packages.SkillBundles(ctx, retained)
	if err != nil {
		return nil, err
	}
	if repository == nil && user == nil && len(bundles) == 0 && (sources.Capture == nil || sources.Capture.Command == "") {
		return nil, nil
	}
	set := candidates{readers: make(map[skilllearning.Choice]selectedSkill), installed: make(map[string]bool)}
	set.input.Library = fmt.Sprintf("%x", sha256.Sum256([]byte(workspaceRoot)))
	if err := set.bundles(bundles, packages); err != nil {
		return nil, err
	}
	if err := set.directories(ctx, user, repository); err != nil {
		return nil, err
	}
	return set.selectReaders(ctx, sources)
}

func (s *resolvedSkills) names() []string {
	return slices.Sorted(maps.Keys(s.byName))
}

func (s *resolvedSkills) find(ctx context.Context, name string) (selectedSkill, error) {
	if err := skillSourceContextError(ctx, skillSourceSelect); err != nil {
		return selectedSkill{}, err
	}
	if s == nil {
		return selectedSkill{}, skillspec.ErrSkillNotFound
	}
	selected, found := s.byName[name]
	if !found {
		return selectedSkill{}, skillspec.ErrSkillNotFound
	}
	if selected.source == nil {
		return selectedSkill{}, errors.Join(workspaceapp.ErrSkillUnavailable, errPackageSkillConflict)
	}
	return selected, nil
}

func (s *resolvedSkills) dependencies() []plugin.Dependency {
	var result []plugin.Dependency
	for _, selected := range s.byName {
		if dependency, installed := selected.origin.Installation(); installed && selected.source != nil {
			result = append(result, dependency)
		}
	}
	return plugin.CompactDependencies(result)
}

func (s *runtimeSkillOverlay) modelSource(ctx context.Context, name string) (skillspec.ResourceSource, error) {
	selected, err := s.selected.find(ctx, name)
	if err != nil {
		return nil, err
	}
	if selected.origin.Scope() == domainskills.ScopeUser {
		return s.decorateUser(selected.source), nil
	}
	return selected.source, nil
}
