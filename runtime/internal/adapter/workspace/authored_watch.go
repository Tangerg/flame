package workspace

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileobservation"
)

const (
	authoredHooksKey  = "hooks"
	authoredSkillsKey = "skills"
)

// AuthoredWatcher maps the Hooks and Skills filesystem layouts onto the
// workspace application's semantic observation port. Global roots are fixed at
// process composition; request-owned workspace roots arrive from Application.
type AuthoredWatcher struct {
	hooksHome  string
	skillsHome string
}

var _ workspaceapp.AuthoredResourceWatcher = AuthoredWatcher{}

// NewAuthoredWatcher binds the global Hooks and Skills roots
// explicitly. An empty Skills root disables its global observation;
// project sources remain observable from request scopes.
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

// Watch observes only the selected resources. Filenames and cascade expansion
// belong to this filesystem translation; consumers see semantic resources.
func (a AuthoredWatcher) Watch(
	scopes []workspaceapp.AuthoredScope,
	resources []workspaceapp.AuthoredResource,
	notify func(workspaceapp.AuthoredResource),
) (workspaceapp.AuthoredObservation, error) {
	targets := make([]fileobservation.Target, 0, 1+len(scopes)*2)
	if slices.Contains(resources, workspaceapp.AuthoredHooks) {
		targets = append(targets, fileobservation.Target{
			Key: authoredHooksKey, Path: filepath.Join(a.hooksHome, ".flame", "hooks.json"),
			MaxBytes: domainhooks.MaxConfigurationFileBytes,
		})
		for _, scope := range scopes {
			directories, err := directoriesRootToLeaf(scope.ProjectRoot, scope.Workspace)
			if err != nil {
				return nil, err
			}
			for _, directory := range directories {
				targets = append(targets, fileobservation.Target{
					Key: authoredHooksKey, Path: filepath.Join(directory, ".flame", "hooks.json"),
					MaxBytes: domainhooks.MaxConfigurationFileBytes,
				})
			}
		}
	}
	report := func(err error) {
		slog.Error("workspace: authored resource observation failed", "error", err)
	}
	files, err := fileobservation.Watch(targets, func(keys []string) {
		for _, key := range keys {
			switch key {
			case authoredHooksKey:
				notify(workspaceapp.AuthoredHooks)
			}
		}
	}, report)
	if err != nil {
		return nil, err
	}
	directoryFiles, err := fileobservation.WatchChildFiles(a.directoryFileTargets(scopes, resources), func(keys []string) {
		for _, key := range keys {
			resource := workspaceapp.AuthoredResource(key)
			if resource.Valid() {
				notify(resource)
			}
		}
	}, report)
	if err != nil {
		return nil, errors.Join(err, files.Close())
	}
	return &authoredObservation{observations: []fileobservation.Observation{files, directoryFiles}}, nil
}

type authoredObservation struct {
	observations []fileobservation.Observation
}

func (a *authoredObservation) Close() error {
	var errs []error
	for _, observation := range a.observations {
		errs = append(errs, observation.Close())
	}
	return errors.Join(errs...)
}

func (a *authoredObservation) Accept(changes []workspaceapp.AuthoredChange) error {
	keys := make([]string, 0, len(changes))
	identities := make([]string, 0, len(changes))
	for _, change := range changes {
		switch change.Resource {
		case workspaceapp.AuthoredHooks:
			keys = append(keys, authoredHooksKey)
		case workspaceapp.AuthoredSkills:
			keys = append(keys, authoredSkillsKey)
		}
		identities = append(identities, change.Identities...)
	}
	var errs []error
	for _, observation := range a.observations {
		errs = append(errs, observation.Accept(keys, identities))
	}
	return errors.Join(errs...)
}

func (a AuthoredWatcher) directoryFileTargets(
	scopes []workspaceapp.AuthoredScope,
	resources []workspaceapp.AuthoredResource,
) []fileobservation.ChildFileTarget {
	targets := make([]fileobservation.ChildFileTarget, 0, 2*(len(scopes)+1))
	if !slices.Contains(resources, workspaceapp.AuthoredSkills) {
		return targets
	}
	if a.skillsHome != "" {
		targets = append(targets, fileobservation.ChildFileTarget{
			Key: authoredSkillsKey, Path: a.skillsHome, Boundary: a.skillsHome, FileName: skillspec.SkillFile,
			MaxEntries: domainskills.MaxSkillDirectoryEntries,
			MaxBytes:   domainskills.MaxAuthoredSkillDocumentBytes,
		})
	}
	for _, scope := range scopes {
		targets = append(targets, fileobservation.ChildFileTarget{
			Key:  authoredSkillsKey,
			Path: promptsource.ProjectSkillDir(scope.Workspace), Boundary: scope.Workspace,
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
		return nil, fmt.Errorf("workspace authored watcher: workspace %q is outside project root %q", leaf, root)
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
