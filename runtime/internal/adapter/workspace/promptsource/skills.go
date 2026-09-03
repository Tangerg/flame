package promptsource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	sdk "github.com/Tangerg/scope/skills"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
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

// MergeSkillSource builds the merged skill source: the selected workspace's
// project directory layered over userDir, the project copy winning on name
// collisions. Returns nil when
// neither directory exists, so a session that ships no skills gets no skill tool
// at all rather than one that always lists nothing.
//
// decorateUser, when non-nil, wraps the USER source only (e.g. to record
// loads for the idle-lifecycle curator). It must not wrap the project source:
// only the user library is auto-curated, and merge resolves a shadowed
// name to the project copy, so decorating the user source records exactly the
// user-resolved loads and nothing else.
//
// Building a source resolves its physical confinement root and wraps it with
// Scope's directory repository, so it remains cheap enough to call per tool
// resolution.
func MergeSkillSource(workspaceRoot, userDir string, decorateUser func(sdk.ResourceSource) sdk.ResourceSource) (sdk.ResourceSource, error) {
	layers, err := openRuntimeSkillLayers(workspaceRoot, userDir)
	if err != nil {
		return nil, err
	}
	return layers.merge(decorateUser), nil
}

func (l runtimeSkillLayers) merge(decorateUser func(sdk.ResourceSource) sdk.ResourceSource) sdk.ResourceSource {
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
	return sdk.Merge(sources...)
}

// ListSkills enumerates the skills visible from the selected workspace layered
// over userDir, project winning on a name collision (the same precedence
// MergeSkillSource gives the model). A missing directory contributes nothing
// rather than erroring. The source projection resolves precedence and preserves
// encounter order; Application owns public catalog order.
func ListSkills(ctx context.Context, workspaceRoot, userDir string) ([]workspaceapp.SkillSummary, error) {
	layers, err := openRuntimeSkillLayers(workspaceRoot, userDir)
	if err != nil {
		return nil, err
	}
	return layers.list(ctx)
}

func (l runtimeSkillLayers) list(ctx context.Context) ([]workspaceapp.SkillSummary, error) {
	seen := make(map[string]struct{})
	var out []workspaceapp.SkillSummary
	for _, layer := range []struct {
		source *runtimeSkillSource
		scope  workspaceapp.SkillScope
	}{
		{source: l.project, scope: workspaceapp.SkillScopeProject},
		{source: l.user, scope: workspaceapp.SkillScopeUser},
	} {
		if layer.source == nil {
			continue
		}
		summaries, err := layer.source.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, summary := range summaries {
			if _, duplicate := seen[summary.Name]; duplicate {
				continue
			}
			seen[summary.Name] = struct{}{}
			out = append(out, workspaceapp.SkillSummary{
				Name: summary.Name, Description: summary.Description, Scope: layer.scope,
			})
		}
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
