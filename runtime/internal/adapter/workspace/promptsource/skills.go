package promptsource

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	sdk "github.com/Tangerg/scope/skills"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
)

const projectSkillsSubdir = ".flame/skills"

// ProjectSkillDir resolves the project skill-source directory for a selected
// workspace. The .flame layout is a prompt-source filesystem convention, not
// a skills-domain concern.
func ProjectSkillDir(workspaceRoot string) string {
	if workspaceRoot == "" {
		return ""
	}
	return filepath.Join(workspaceRoot, projectSkillsSubdir)
}

// OverlaySkillSource builds the overlaid skill source: the selected workspace's
// project directory layered over userDir, the project copy winning on name
// collisions. Returns nil when
// neither directory exists, so a session that ships no skills gets no skill tool
// at all rather than one that always lists nothing.
//
// decorateUser, when non-nil, wraps the USER source only (e.g. to record
// loads for the idle-lifecycle curator). It must not wrap the project source:
// only the user library is auto-curated, and the overlay resolves a shadowed
// name to the project copy, so decorating the user source records exactly the
// user-resolved loads and nothing else.
//
// Building a source resolves its physical confinement root and wraps it with
// Scope's directory repository, so it remains cheap enough to call per tool
// resolution.
func OverlaySkillSource(ctx context.Context, workspaceRoot, userDir string, packages PackageSkills, decorateUser func(sdk.ResourceSource) sdk.ResourceSource) (sdk.ResourceSource, []InstallationDependency, error) {
	layers, err := openRuntimeSkillLayers(ctx, workspaceRoot, userDir, packages)
	if err != nil {
		return nil, nil, err
	}
	dependencies, err := layers.dependencies(ctx)
	if err != nil {
		return nil, nil, err
	}
	return layers.overlay(decorateUser), dependencies, nil
}

func (l runtimeSkillLayers) overlay(decorateUser func(sdk.ResourceSource) sdk.ResourceSource) sdk.ResourceSource {
	if l.project == nil && l.user == nil && len(l.packages) == 0 {
		return nil
	}
	return &runtimeSkillOverlay{layers: l, decorateUser: decorateUser}
}

// ListSkills enumerates the skills visible from the selected workspace layered
// over userDir, project winning on a name collision (the same precedence
// OverlaySkillSource gives the model). A missing directory contributes nothing
// rather than erroring. Malformed selected documents produce diagnostics;
// unrelated I/O failures still fail the query.
func ListSkills(ctx context.Context, workspaceRoot, userDir string, packages PackageSkills) (workspaceapp.SkillDiscovery, error) {
	layers, err := openRuntimeSkillLayers(ctx, workspaceRoot, userDir, packages)
	if err != nil {
		return workspaceapp.SkillDiscovery{}, err
	}
	return layers.list(ctx)
}

// Discovery and execution use the same precedence-resolved source, including
// when a broken higher-precedence document hides an otherwise valid copy.
type runtimeSkillOverlay struct {
	layers       runtimeSkillLayers
	decorateUser func(sdk.ResourceSource) sdk.ResourceSource
}

func (s *runtimeSkillOverlay) List(ctx context.Context) ([]sdk.Summary, error) {
	catalog, err := s.layers.list(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]sdk.Summary, 0, len(catalog.Skills))
	for _, entry := range catalog.Skills {
		summaries = append(summaries, sdk.Summary{Name: entry.Name, Description: entry.Description})
	}
	return summaries, nil
}

func (l runtimeSkillLayers) get(ctx context.Context, name string) (workspaceapp.SkillDetail, error) {
	selected, err := l.modelSource(ctx, name, nil)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	for _, layer := range []struct {
		source *runtimeSkillSource
		origin workspaceapp.SkillSource
	}{
		{l.project, workspaceapp.ProjectSkillSource()}, {l.user, workspaceapp.UserSkillSource()},
	} {
		if layer.source == nil || layer.source != selected {
			continue
		}
		skill, content, err := layer.source.document(ctx, name)
		if err != nil {
			return workspaceapp.SkillDetail{}, err
		}
		digest := sha256.Sum256(content)
		return workspaceapp.SkillDetail{
			SkillSummary: workspaceapp.SkillSummary{Name: skill.Name, Description: skill.Description, Source: layer.origin},
			Path:         filepath.Join(layer.source.root, name, sdk.SkillFile), Revision: fmt.Sprintf("%x", digest), Instructions: skill.Instructions,
		}, nil
	}
	packageSource, err := l.packageSource(name)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	origin, err := workspaceapp.InstallationSkillSource(packageSource.bundle.Dependency.InstallationID, packageSource.bundle.Dependency.Digest)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	skill, content, err := packageSource.document(ctx, name)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	digest := sha256.Sum256(content)
	return workspaceapp.SkillDetail{
		SkillSummary: workspaceapp.SkillSummary{Name: skill.Name, Description: skill.Description, Source: origin},
		Path:         filepath.Join(packageSource.bundle.Root, name, sdk.SkillFile), Revision: fmt.Sprintf("%x", digest), Instructions: skill.Instructions,
	}, nil
}

func (l runtimeSkillLayers) list(ctx context.Context) (workspaceapp.SkillDiscovery, error) {
	names := make(map[string]struct{})
	for _, source := range []*runtimeSkillSource{l.project, l.user} {
		if source == nil {
			continue
		}
		entries, err := source.directoryEntries(ctx)
		if err != nil {
			return workspaceapp.SkillDiscovery{}, err
		}
		for _, name := range skillCandidateNames(entries) {
			names[name] = struct{}{}
		}
	}
	for _, source := range l.packages {
		for _, name := range source.bundle.Names {
			names[name] = struct{}{}
		}
	}
	out := workspaceapp.SkillDiscovery{Skills: []workspaceapp.SkillSummary{}, Diagnostics: []workspaceapp.SkillDiagnostic{}}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		detail, err := l.get(ctx, name)
		if context.Cause(ctx) != nil {
			return workspaceapp.SkillDiscovery{}, context.Cause(ctx)
		}
		switch {
		case errors.Is(err, errPackageSkillConflict):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Multiple installations supply this Skill; disable a package copy or provide an explicit project/user override."})
			continue
		case errors.Is(err, sdk.ErrInvalidSkill):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Invalid SKILL.md; repair the selected bundle's frontmatter and name."})
			continue
		case errors.Is(err, domainskills.ErrDocumentTooLarge):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "SKILL.md exceeds the 1 MiB document limit; move supporting material into resource files."})
			continue
		case errors.Is(err, workspaceapp.ErrSkillUnavailable):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Installed Skill unavailable; verify its selected release and current installation approval."})
			continue
		case errors.Is(err, sdk.ErrSkillNotFound):
			continue
		case err != nil:
			return workspaceapp.SkillDiscovery{}, err
		}
		if _, _, installed := detail.Source.Installation(); !installed && slices.ContainsFunc(l.packages, func(source packageSkillSource) bool { return slices.Contains(source.bundle.Names, name) }) {
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "An explicit project/user Skill overrides the installed package copy."})
		}
		out.Skills = append(out.Skills, detail.SkillSummary)
	}
	return out, out.Validate()
}

type runtimeSkillLayers struct {
	packages []packageSkillSource
	project  *runtimeSkillSource
	user     *runtimeSkillSource
}

func openRuntimeSkillLayers(ctx context.Context, workspaceRoot, userDir string, packages PackageSkills) (runtimeSkillLayers, error) {
	project, err := openRuntimeSkillSource(ProjectSkillDir(workspaceRoot), workspaceRoot)
	if err != nil {
		return runtimeSkillLayers{}, err
	}
	user, err := openRuntimeSkillSource(userDir, userDir)
	if err != nil {
		return runtimeSkillLayers{}, err
	}
	layers := runtimeSkillLayers{project: project, user: user}
	if packages != nil {
		bundles, err := packages.SkillBundles(ctx)
		if err != nil {
			return layers, err
		}
		for _, bundle := range bundles {
			layers.packages = append(layers.packages, packageSkillSource{bundle: bundle, authority: packages})
		}
	}
	return layers, nil
}

func openRuntimeSkillSource(root, boundary string) (*runtimeSkillSource, error) {
	present, err := skillSourceDirectory(root)
	if err != nil || !present {
		return nil, err
	}
	return newRuntimeSkillSource(root, boundary)
}

// skillSourceDirectory distinguishes an absent optional source from a broken
// higher-precedence source. Existing aliases and non-directories must not be
// silently converted into absence and expose a lower-precedence catalog.
func skillSourceDirectory(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("promptsource: inspect Skill source %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		info, err = os.Stat(path)
		if err != nil {
			return false, fmt.Errorf("promptsource: resolve Skill source %q: %w", path, err)
		}
	}
	if !info.IsDir() {
		return false, fmt.Errorf("promptsource: Skill source %q is not a directory", path)
	}
	return true, nil
}
