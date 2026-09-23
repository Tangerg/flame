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
func OverlaySkillSource(workspaceRoot, userDir string, decorateUser func(sdk.ResourceSource) sdk.ResourceSource) (sdk.ResourceSource, error) {
	layers, err := openRuntimeSkillLayers(workspaceRoot, userDir)
	if err != nil {
		return nil, err
	}
	return layers.overlay(decorateUser), nil
}

func (l runtimeSkillLayers) overlay(decorateUser func(sdk.ResourceSource) sdk.ResourceSource) sdk.ResourceSource {
	sources := make([]sdk.ResourceSource, 0, 2)
	if l.project != nil {
		sources = append(sources, l.project)
	}
	if l.user != nil {
		var user sdk.ResourceSource = l.user
		if decorateUser != nil {
			user = decorateUser(user)
		}
		sources = append(sources, user)
	}
	if len(sources) == 0 {
		return nil
	}
	return &runtimeSkillOverlay{ResourceSource: sdk.Overlay(sources...), layers: l}
}

// ListSkills enumerates the skills visible from the selected workspace layered
// over userDir, project winning on a name collision (the same precedence
// OverlaySkillSource gives the model). A missing directory contributes nothing
// rather than erroring. Malformed selected documents produce diagnostics;
// unrelated I/O failures still fail the query.
func ListSkills(ctx context.Context, workspaceRoot, userDir string) (workspaceapp.SkillDiscovery, error) {
	layers, err := openRuntimeSkillLayers(workspaceRoot, userDir)
	if err != nil {
		return workspaceapp.SkillDiscovery{}, err
	}
	return layers.list(ctx)
}

// The SDK owns name precedence. This projection adds partial discovery without
// advertising a lower-precedence bundle when the selected document is broken.
type runtimeSkillOverlay struct {
	sdk.ResourceSource
	layers runtimeSkillLayers
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

type inspectedSkillSource struct {
	*runtimeSkillSource
	scope  domainskills.Scope
	detail *workspaceapp.SkillDetail
}

// Capture provenance from the source actually chosen by SDK Load, using the
// same bounded and version-checked document read as model execution.
func (s *inspectedSkillSource) Load(ctx context.Context, name string) (*sdk.Skill, error) {
	skill, content, err := s.document(ctx, name)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(content)
	s.detail = &workspaceapp.SkillDetail{
		SkillSummary: workspaceapp.SkillSummary{Name: skill.Name, Description: skill.Description, Scope: s.scope},
		Path:         filepath.Join(s.root, name, sdk.SkillFile), Revision: fmt.Sprintf("%x", digest), Instructions: skill.Instructions,
	}
	return skill, nil
}

func (l runtimeSkillLayers) get(ctx context.Context, name string) (workspaceapp.SkillDetail, error) {
	var sources []sdk.ResourceSource
	var inspected []*inspectedSkillSource
	for _, layer := range []struct {
		source *runtimeSkillSource
		scope  domainskills.Scope
	}{
		{l.project, domainskills.ScopeProject}, {l.user, domainskills.ScopeUser},
	} {
		if layer.source == nil {
			continue
		}
		source := &inspectedSkillSource{runtimeSkillSource: layer.source, scope: layer.scope}
		sources = append(sources, source)
		inspected = append(inspected, source)
	}
	if _, err := sdk.Overlay(sources...).Load(ctx, name); err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	for _, source := range inspected {
		if source.detail != nil {
			return *source.detail, nil
		}
	}
	return workspaceapp.SkillDetail{}, fmt.Errorf("runtime skill source: resolver returned no document")
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
	out := workspaceapp.SkillDiscovery{Skills: []workspaceapp.SkillSummary{}, Diagnostics: []workspaceapp.SkillDiagnostic{}}
	counts := make(map[domainskills.Scope]int)
	for _, name := range slices.Sorted(maps.Keys(names)) {
		detail, err := l.get(ctx, name)
		if context.Cause(ctx) != nil {
			return workspaceapp.SkillDiscovery{}, context.Cause(ctx)
		}
		switch {
		case errors.Is(err, sdk.ErrInvalidSkill):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Invalid SKILL.md; repair the selected bundle's frontmatter and name."})
			continue
		case errors.Is(err, domainskills.ErrDocumentTooLarge):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "SKILL.md exceeds the 1 MiB document limit; move supporting material into resource files."})
			continue
		case errors.Is(err, sdk.ErrSkillNotFound):
			continue
		case err != nil:
			return workspaceapp.SkillDiscovery{}, err
		}
		counts[detail.Scope]++
		if counts[detail.Scope] > domainskills.MaxSkillsPerSource {
			return workspaceapp.SkillDiscovery{}, fmt.Errorf("%w: selected source exceeds %d Skills", domainskills.ErrLibraryCapacity, domainskills.MaxSkillsPerSource)
		}
		out.Skills = append(out.Skills, detail.SkillSummary)
	}
	return out, nil
}

type runtimeSkillLayers struct {
	project *runtimeSkillSource
	user    *runtimeSkillSource
}

func openRuntimeSkillLayers(workspaceRoot, userDir string) (runtimeSkillLayers, error) {
	project, err := openRuntimeSkillSource(ProjectSkillDir(workspaceRoot), workspaceRoot)
	if err != nil {
		return runtimeSkillLayers{}, err
	}
	user, err := openRuntimeSkillSource(userDir, userDir)
	if err != nil {
		return runtimeSkillLayers{}, err
	}
	return runtimeSkillLayers{project: project, user: user}, nil
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
