package promptsource

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	var sources []sdk.ResourceSource
	projectDir := ProjectSkillDir(workspaceRoot)
	if dirExists(projectDir) {
		project, err := newRuntimeSkillSource(projectDir, workspaceRoot)
		if err != nil {
			return nil, err
		}
		sources = append(sources, project)
	}
	if dirExists(userDir) {
		userSource, err := newRuntimeSkillSource(userDir, userDir)
		if err != nil {
			return nil, err
		}
		var user sdk.ResourceSource = userSource
		if decorateUser != nil {
			user = decorateUser(user)
		}
		sources = append(sources, user)
	}
	if len(sources) == 0 {
		return nil, nil
	}
	return sdk.Merge(sources...), nil
}

// ListSkills enumerates the skills visible from the selected workspace layered
// over userDir, project winning on a name collision (the same precedence
// MergeSkillSource gives the model). A missing directory contributes nothing
// rather than erroring. Result is sorted by name.
func ListSkills(ctx context.Context, workspaceRoot, userDir string) ([]workspaceapp.SkillSummary, error) {
	seen := make(map[string]struct{})
	var out []workspaceapp.SkillSummary
	add := func(dir, boundary string, scope workspaceapp.SkillScope) error {
		if !dirExists(dir) {
			return nil
		}
		source, err := newRuntimeSkillSource(dir, boundary)
		if err != nil {
			return err
		}
		summaries, err := source.List(ctx)
		if err != nil {
			return err
		}
		for _, s := range summaries {
			if _, dup := seen[s.Name]; dup {
				continue // a higher-precedence (project) source already provided it
			}
			seen[s.Name] = struct{}{}
			out = append(out, workspaceapp.SkillSummary{Name: s.Name, Description: s.Description, Scope: scope})
		}
		return nil
	}
	if err := add(ProjectSkillDir(workspaceRoot), workspaceRoot, workspaceapp.SkillScopeProject); err != nil {
		return nil, err
	}
	if err := add(userDir, userDir, workspaceapp.SkillScopeUser); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b workspaceapp.SkillSummary) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// dirExists reports whether path names an existing directory.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
