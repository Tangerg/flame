package promptsource

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	skillspec "github.com/Tangerg/scope/skills"
)

const (
	repositorySkillsSubdir = identity.AuthoredDirectory + "/skills"
)

func RepositorySkillDir(workspaceRoot string) string {
	if workspaceRoot == "" {
		return ""
	}
	return filepath.Join(workspaceRoot, repositorySkillsSubdir)
}

func OverlaySkillSource(ctx context.Context, workspaceRoot, userDir string, packages SkillSources, decorateUser func(skillspec.ResourceSource) skillspec.ResourceSource, retained []plugin.Dependency) (skillspec.ResourceSource, []plugin.Dependency, error) {
	if decorateUser == nil {
		return nil, nil, errors.New("skills: user source decorator is required")
	}
	selected, err := resolveSkills(ctx, workspaceRoot, userDir, packages, retained)
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

func ListSkills(ctx context.Context, workspaceRoot, userDir string, packages SkillSources) (workspaceapp.SkillDiscovery, error) {
	selected, err := resolveSkills(ctx, workspaceRoot, userDir, packages, nil)
	if err != nil {
		return workspaceapp.SkillDiscovery{}, err
	}
	return selected.list(ctx)
}

type runtimeSkillOverlay struct {
	selected     *resolvedSkills
	decorateUser func(skillspec.ResourceSource) skillspec.ResourceSource
}

func (s *runtimeSkillOverlay) List(ctx context.Context) ([]skillspec.Summary, error) {
	catalog, err := s.selected.list(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]skillspec.Summary, 0, len(catalog.Skills))
	for _, entry := range catalog.Skills {
		summaries = append(summaries, skillspec.Summary{Name: entry.Name, Description: entry.Description})
	}
	return summaries, nil
}

func (s *runtimeSkillOverlay) Lookup(ctx context.Context, name string) (skillspec.Summary, error) {
	source, err := s.modelSource(ctx, name)
	if err != nil {
		return skillspec.Summary{}, err
	}
	return source.Lookup(ctx, name)
}

func (s *runtimeSkillOverlay) Load(ctx context.Context, name string) (*skillspec.Skill, error) {
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
	if err := skillSourceContextError(ctx, skillSourceList); err != nil {
		return workspaceapp.SkillDiscovery{}, err
	}
	out := workspaceapp.SkillDiscovery{Skills: []workspaceapp.SkillSummary{}, Diagnostics: []workspaceapp.SkillDiagnostic{}}
	if s == nil {
		return out, nil
	}
	for _, name := range s.names() {
		detail, err := s.get(ctx, name)
		if cancelErr := skillSourceContextError(ctx, skillSourceList); cancelErr != nil {
			return workspaceapp.SkillDiscovery{}, errors.Join(err, cancelErr)
		}
		switch {
		case errors.Is(err, errPackageSkillConflict):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Multiple installations supply this Skill; disable a package copy or provide an explicit repository/user override."})
			continue
		case errors.Is(err, skillspec.ErrInvalidSkill):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Invalid SKILL.md; provide a regular document with valid frontmatter and the selected bundle's name."})
			continue
		case errors.Is(err, domainskills.ErrDocumentTooLarge):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "SKILL.md exceeds the 1 MiB document limit; move supporting material into resource files."})
			continue
		case errors.Is(err, workspaceapp.ErrSkillUnavailable):
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "Installed Skill unavailable; verify its selected release and current installation approval."})
			continue
		case errors.Is(err, skillspec.ErrSkillNotFound):
			continue
		case err != nil:
			return workspaceapp.SkillDiscovery{}, err
		}
		if _, installed := detail.Source.Installation(); !installed && s.byName[name].packageCopy {
			out.Diagnostics = append(out.Diagnostics, workspaceapp.SkillDiagnostic{Name: name, Detail: "An explicit repository/user Skill overrides the installed package copy."})
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

// Existing aliases and non-directories must not silently become absence and expose
// a lower-precedence catalog.
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
