package promptsource

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skilldocument"
	skillspec "github.com/Tangerg/scope/skills"
)

type PackageSkillBundle struct {
	Root       string
	Dependency plugin.Dependency
	Skills     []plugin.Skill
}
type PackageSkills interface {
	SkillBundles(context.Context, []plugin.Dependency) ([]PackageSkillBundle, error)
	ReadSkillResource(context.Context, plugin.Dependency, string, string) ([]byte, error)
}
type packageSkillSource struct {
	bundle    PackageSkillBundle
	authority PackageSkills
}

func (s *packageSkillSource) Lookup(ctx context.Context, name string) (skillspec.Summary, error) {
	if err := skillSourceContextError(ctx, skillSourceLookup); err != nil {
		return skillspec.Summary{}, err
	}
	index := slices.IndexFunc(s.bundle.Skills, func(skill plugin.Skill) bool { return skill.Name == name })
	if index < 0 {
		return skillspec.Summary{}, skillspec.ErrSkillNotFound
	}
	skill := s.bundle.Skills[index]
	return skillspec.Summary{Name: skill.Name, Description: skill.Description}, nil
}

func (s *packageSkillSource) document(ctx context.Context, name string) (*skillspec.Skill, []byte, error) {
	if err := skillSourceContextError(ctx, skillSourceLoad); err != nil {
		return nil, nil, err
	}
	content, err := s.authority.ReadSkillResource(ctx, s.bundle.Dependency, name, skillspec.SkillFile)
	if cancelErr := skillSourceContextError(ctx, skillSourceLoad); cancelErr != nil {
		return nil, nil, errors.Join(err, cancelErr)
	}
	if err != nil {
		return nil, nil, err
	}
	if len(content) > domainskills.MaxAuthoredSkillDocumentBytes {
		return nil, nil, domainskills.ErrDocumentTooLarge
	}
	skill, err := skilldocument.Load(ctx, name, content)
	return skill, content, err
}

func (s *packageSkillSource) Load(ctx context.Context, name string) (*skillspec.Skill, error) {
	skill, _, err := s.document(ctx, name)
	return skill, err
}

func (s *packageSkillSource) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	if err := skillSourceContextError(ctx, skillSourceOpenResource); err != nil {
		return nil, err
	}
	content, err := s.authority.ReadSkillResource(ctx, s.bundle.Dependency, name, resource)
	if cancelErr := skillSourceContextError(ctx, skillSourceOpenResource); cancelErr != nil {
		return nil, errors.Join(err, cancelErr)
	}
	if err != nil {
		return nil, err
	}
	if len(content) > domainskills.MaxSkillResourceBytes {
		return nil, domainskills.ErrResourceTooLarge
	}
	return &skillResourceBytes{Reader: bytes.NewReader(content), name: resource}, nil
}

type skillResourceBytes struct {
	*bytes.Reader
	name string
}

func (f *skillResourceBytes) Close() error               { return nil }
func (f *skillResourceBytes) Stat() (fs.FileInfo, error) { return f, nil }
func (f *skillResourceBytes) Name() string               { return f.name }
func (f *skillResourceBytes) Size() int64                { return f.Reader.Size() }
func (f *skillResourceBytes) Mode() fs.FileMode          { return filesystem.ReadOnlyFileMode }
func (f *skillResourceBytes) ModTime() time.Time         { return time.Time{} }
func (f *skillResourceBytes) IsDir() bool                { return false }
func (f *skillResourceBytes) Sys() any                   { return nil }

func (s *packageSkillSource) List(ctx context.Context) ([]skillspec.Summary, error) {
	if err := skillSourceContextError(ctx, skillSourceList); err != nil {
		return nil, err
	}
	result := make([]skillspec.Summary, 0, len(s.bundle.Skills))
	for _, skill := range s.bundle.Skills {
		result = append(result, skillspec.Summary{Name: skill.Name, Description: skill.Description})
	}
	return result, nil
}

func (s *packageSkillSource) documentPath(name string) string {
	return filepath.Join(s.bundle.Root, name, skillspec.SkillFile)
}
