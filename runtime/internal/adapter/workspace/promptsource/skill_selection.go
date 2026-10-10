package promptsource

import (
	"context"
	"errors"
	"maps"
	"slices"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	sdk "github.com/Tangerg/scope/skills"
)

var errPackageSkillConflict = errors.New("promptsource: package Skill name conflict")

type skillDocumentSource interface {
	sdk.ResourceSource
	document(context.Context, string) (*sdk.Skill, []byte, error)
	documentPath(string) string
}

type selectedSkill struct {
	source      skillDocumentSource // nil retains a package conflict until the next resolution.
	origin      workspaceapp.SkillSource
	packageCopy bool
}

// Selection is immutable for its consumer. Every callable resource, including
// one whose document is temporarily unavailable, retains its release dependency.
type resolvedSkills struct {
	byName map[string]selectedSkill
}

func resolveSkills(ctx context.Context, workspaceRoot, userDir string, packages PackageSkills) (*resolvedSkills, error) {
	project, err := openRuntimeSkillSource(ProjectSkillDir(workspaceRoot), workspaceRoot)
	if err != nil {
		return nil, err
	}
	user, err := openRuntimeSkillSource(userDir, userDir)
	if err != nil {
		return nil, err
	}
	var bundles []PackageSkillBundle
	if packages != nil {
		bundles, err = packages.SkillBundles(ctx)
		if err != nil {
			return nil, err
		}
	}
	if project == nil && user == nil && len(bundles) == 0 {
		return nil, nil
	}
	selected := &resolvedSkills{byName: make(map[string]selectedSkill)}
	for _, bundle := range bundles {
		origin, err := workspaceapp.InstallationSkillSource(bundle.Dependency)
		if err != nil {
			return nil, err
		}
		source := &packageSkillSource{bundle: bundle, authority: packages}
		for _, skill := range bundle.Skills {
			if _, found := selected.byName[skill.Name]; found {
				selected.byName[skill.Name] = selectedSkill{packageCopy: true}
				continue
			}
			selected.byName[skill.Name] = selectedSkill{source: source, origin: origin, packageCopy: true}
		}
	}
	for _, layer := range []struct {
		source *SkillSource
		origin workspaceapp.SkillSource
	}{
		{user, workspaceapp.UserSkillSource()}, {project, workspaceapp.ProjectSkillSource()},
	} {
		if layer.source == nil {
			continue
		}
		entries, err := layer.source.directoryEntries(ctx)
		if err != nil {
			return nil, err
		}
		for _, name := range skillCandidateNames(entries) {
			selected.byName[name] = selectedSkill{source: layer.source, origin: layer.origin, packageCopy: selected.byName[name].packageCopy}
		}
	}
	return selected, nil
}

func (s *resolvedSkills) names() []string {
	return slices.Sorted(maps.Keys(s.byName))
}

func (s *resolvedSkills) find(ctx context.Context, name string) (selectedSkill, error) {
	if err := skillSourceContextError(ctx, "select"); err != nil {
		return selectedSkill{}, err
	}
	if s == nil {
		return selectedSkill{}, sdk.ErrSkillNotFound
	}
	selected, found := s.byName[name]
	if !found {
		return selectedSkill{}, sdk.ErrSkillNotFound
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

func (s *runtimeSkillOverlay) modelSource(ctx context.Context, name string) (sdk.ResourceSource, error) {
	selected, err := s.selected.find(ctx, name)
	if err != nil {
		return nil, err
	}
	if selected.origin.Scope() == domainskills.ScopeUser && s.decorateUser != nil {
		return s.decorateUser(selected.source), nil
	}
	return selected.source, nil
}
