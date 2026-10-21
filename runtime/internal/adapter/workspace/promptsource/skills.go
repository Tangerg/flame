package promptsource

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	sdk "github.com/Tangerg/scope/skills"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
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

// OverlaySkillSource resolves sources once for an execution. Its callable names
// and installation dependencies share that selection; later directory changes
// cannot add a name or replace its authority. Selected documents remain editable.
// decorateUser records only loads selected from the user library.
func OverlaySkillSource(ctx context.Context, workspaceRoot, userDir string, packages PackageSkills, decorateUser func(sdk.ResourceSource) sdk.ResourceSource) (sdk.ResourceSource, []plugin.Dependency, error) {
	selected, err := resolveSkills(ctx, workspaceRoot, userDir, packages)
	if err != nil {
		return nil, nil, err
	}
	if selected == nil {
		return nil, nil, nil
	}
	if _, err := selected.list(ctx); err != nil {
		return nil, nil, err
	}
	return &runtimeSkillOverlay{selected: selected, decorateUser: decorateUser}, selected.dependencies(), nil
}

// ListSkills enumerates the skills visible from the selected workspace layered
// over userDir, project winning on a name collision (the same precedence
// OverlaySkillSource gives the model). A missing directory contributes nothing
// rather than erroring. Malformed selected documents produce diagnostics;
// unrelated I/O failures still fail the query.
func ListSkills(ctx context.Context, workspaceRoot, userDir string, packages PackageSkills) (workspaceapp.SkillDiscovery, error) {
	selected, err := resolveSkills(ctx, workspaceRoot, userDir, packages)
	if err != nil {
		return workspaceapp.SkillDiscovery{}, err
	}
	return selected.list(ctx)
}

// Discovery and execution use the same precedence-resolved source, including
// when a broken higher-precedence document hides an otherwise valid copy.
type runtimeSkillOverlay struct {
	selected     *resolvedSkills
	decorateUser func(sdk.ResourceSource) sdk.ResourceSource
}

func (s *runtimeSkillOverlay) List(ctx context.Context) ([]sdk.Summary, error) {
	catalog, err := s.selected.list(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]sdk.Summary, 0, len(catalog.Skills))
	for _, entry := range catalog.Skills {
		summaries = append(summaries, sdk.Summary{Name: entry.Name, Description: entry.Description})
	}
	return summaries, nil
}

func (s *runtimeSkillOverlay) Lookup(ctx context.Context, name string) (sdk.Summary, error) {
	source, err := s.modelSource(ctx, name)
	if err != nil {
		return sdk.Summary{}, err
	}
	return source.Lookup(ctx, name)
}

func (s *runtimeSkillOverlay) Load(ctx context.Context, name string) (*sdk.Skill, error) {
	source, err := s.modelSource(ctx, name)
	if err != nil {
		return nil, err
	}
	return source.Load(ctx, name)
}

func (s *runtimeSkillOverlay) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	source, err := s.modelSource(ctx, name)
	if err != nil {
		return nil, err
	}
	return source.OpenResource(ctx, name, resource)
}

func (s *resolvedSkills) get(ctx context.Context, name string) (workspaceapp.SkillDetail, error) {
	selected, err := s.find(ctx, name)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	skill, content, err := selected.source.document(ctx, name)
	if err != nil {
		return workspaceapp.SkillDetail{}, err
	}
	digest := sha256.Sum256(content)
	return workspaceapp.SkillDetail{
		SkillSummary: workspaceapp.SkillSummary{Name: skill.Name, Description: skill.Description, Source: selected.origin},
		Path:         selected.source.documentPath(name), Revision: fmt.Sprintf("%x", digest), Instructions: skill.Instructions,
	}, nil
}

func (s *resolvedSkills) list(ctx context.Context) (workspaceapp.SkillDiscovery, error) {
	if err := skillSourceContextError(ctx, "list"); err != nil {
		return workspaceapp.SkillDiscovery{}, err
	}
	out := workspaceapp.SkillDiscovery{Skills: []workspaceapp.SkillSummary{}, Diagnostics: []workspaceapp.SkillDiagnostic{}}
	if s == nil {
		return out, nil
	}
	for _, name := range s.names() {
		detail, err := s.get(ctx, name)
		if cancelErr := skillSourceContextError(ctx, "list"); cancelErr != nil {
			return workspaceapp.SkillDiscovery{}, errors.Join(err, cancelErr)
		}
		switch {
		case errors.Is(err, errPackageSkillConflict):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Multiple installations supply this Skill; disable a package copy or provide an explicit project/user override."})
			continue
		case errors.Is(err, sdk.ErrInvalidSkill):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Invalid SKILL.md; provide a regular document with valid frontmatter and the selected bundle's name."})
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
		if _, installed := detail.Source.Installation(); !installed && s.byName[name].packageCopy {
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "An explicit project/user Skill overrides the installed package copy."})
		}
		out.Skills = append(out.Skills, detail.SkillSummary)
	}
	return out, out.Validate()
}

func openRuntimeSkillSource(root, boundary string) (*SkillSource, error) {
	present, err := skillSourceDirectory(root)
	if err != nil || !present {
		return nil, err
	}
	return NewSkillSource(root, boundary)
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
