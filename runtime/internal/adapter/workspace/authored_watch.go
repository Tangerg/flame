package workspace

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileobservation"
	skillspec "github.com/Tangerg/scope/skills"
)

type AuthoredWatcher struct {
	hooksHome  string
	skillsHome string
}

var _ workspaceapp.AuthoredResourceWatcher = AuthoredWatcher{}

func NewAuthoredWatcher(hooksHome, skillsHome string) (AuthoredWatcher, error) {
	if hooksHome == "" || !filepath.IsAbs(hooksHome) {
		return AuthoredWatcher{}, errors.New("workspace authored watcher: hooks home must be absolute")
	}
	if skillsHome != "" && !filepath.IsAbs(skillsHome) {
		return AuthoredWatcher{}, errors.New("workspace authored watcher: skills home must be absolute when set")
	}
	return AuthoredWatcher{
		hooksHome:  filepath.Clean(hooksHome),
		skillsHome: cleanOptionalPath(skillsHome),
	}, nil
}

func (a AuthoredWatcher) Watch(
	scopes []workspaceapp.AuthoredScope,
	resources []workspaceapp.AuthoredResource,
	notify func(workspaceapp.AuthoredResource),
	report func(error),
) (workspaceapp.AuthoredObservation, error) {
	targets := make([]fileobservation.Target, 0, 1+len(scopes)*2)
	if slices.Contains(resources, workspaceapp.AuthoredHooks) {
		targets = append(targets, fileobservation.Target{
			Key: string(workspaceapp.AuthoredHooks), Path: filepath.Join(a.hooksHome, hookdata.ConfigurationPath),
			MaxBytes: hookdata.MaxConfigurationFileBytes,
		})
		for _, scope := range scopes {
			directories, err := directoriesRootToLeaf(scope.RepositoryRoot, scope.Workspace)
			if err != nil {
				return nil, err
			}
			for _, directory := range directories {
				targets = append(targets, fileobservation.Target{
					Key: string(workspaceapp.AuthoredHooks), Path: filepath.Join(directory, hookdata.ConfigurationPath),
					MaxBytes: hookdata.MaxConfigurationFileBytes,
				})
			}
		}
	}
	hooks, err := fileobservation.Watch(targets, func([]string) { notify(workspaceapp.AuthoredHooks) }, report)
	if err != nil {
		return nil, err
	}
	skills, err := fileobservation.WatchChildFiles(a.skillTargets(scopes, resources), func() { notify(workspaceapp.AuthoredSkills) }, report)
	if err != nil {
		return nil, errors.Join(err, hooks.Close())
	}
	return authoredObservation{hooks: hooks, skills: skills}, nil
}

type authoredObservation struct {
	hooks  io.Closer
	skills fileobservation.ChildFilesObservation
}

func (a authoredObservation) Close() error {
	return errors.Join(a.hooks.Close(), a.skills.Close())
}

func (a authoredObservation) ReconcileSkillPublication(paths []string) error {
	return a.skills.AdvancePublishedPaths(paths)
}

func (a AuthoredWatcher) skillTargets(
	scopes []workspaceapp.AuthoredScope,
	resources []workspaceapp.AuthoredResource,
) []fileobservation.ChildFileTarget {
	targets := make([]fileobservation.ChildFileTarget, 0, 2*(len(scopes)+1))
	if !slices.Contains(resources, workspaceapp.AuthoredSkills) {
		return targets
	}
	if a.skillsHome != "" {
		targets = append(targets, fileobservation.ChildFileTarget{
			Path: a.skillsHome, Boundary: a.skillsHome, FileName: skillspec.SkillFile,
			MaxEntries: domainskills.MaxSkillDirectoryEntries,
			MaxBytes:   domainskills.MaxAuthoredSkillDocumentBytes,
		})
	}
	for _, scope := range scopes {
		targets = append(targets, fileobservation.ChildFileTarget{
			Path: promptsource.RepositorySkillDir(scope.Workspace), Boundary: scope.Workspace,
			FileName: skillspec.SkillFile, MaxEntries: domainskills.MaxSkillDirectoryEntries,
			MaxBytes: domainskills.MaxAuthoredSkillDocumentBytes,
		})
	}
	return targets
}

func cleanOptionalPath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func directoriesRootToLeaf(root, leaf string) ([]string, error) {
	root = filepath.Clean(root)
	leaf = filepath.Clean(leaf)
	relative, err := filepath.Rel(root, leaf)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return nil, fmt.Errorf("workspace authored watcher: workspace %q is outside repository root %q", leaf, root)
	}
	chain := []string{leaf}
	for current := leaf; current != root; {
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	slices.Reverse(chain)
	return chain, nil
}
