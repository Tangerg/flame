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

func resolveSkills(ctx context.Context, workspaceRoot, userDir string, sources SkillSources, retained []plugin.Dependency) (*resolvedSkills, error) {
	project, err := openRuntimeSkillSource(ProjectSkillDir(workspaceRoot), workspaceRoot)
	if err != nil {
		return nil, err
	}
	user, err := openRuntimeSkillSource(userDir, userDir)
	if err != nil {
		return nil, err
	}
	packages := sources.Packages
	var bundles []PackageSkillBundle
	if packages != nil {
		bundles, err = packages.SkillBundles(ctx, retained)
		if err != nil {
			return nil, err
		}
	} else if len(retained) != 0 {
		return nil, workspaceapp.ErrSkillUnavailable
	}
	if project == nil && user == nil && len(bundles) == 0 && (sources.Capture == nil || sources.Capture.Command == "") {
		return nil, nil
	}
	set := candidates{readers: make(map[skilllearning.Choice]selectedSkill), installed: make(map[string]bool)}
	set.input.Library = fmt.Sprintf("%x", sha256.Sum256([]byte(workspaceRoot)))
	if err := set.bundles(bundles, packages); err != nil {
		return nil, err
	}
	if err := set.directories(ctx, user, project); err != nil {
		return nil, err
	}
	return set.selectReaders(ctx, sources)

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
