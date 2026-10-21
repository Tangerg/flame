package promptsource

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skilldocument"
	sdk "github.com/Tangerg/scope/skills"
)

type PackageSkillBundle struct {
	Root       string
	Dependency plugin.Dependency
	Skills     []plugin.Skill
}
type PackageSkills interface {
	SkillBundles(context.Context) ([]PackageSkillBundle, error)
	ReadSkillResource(context.Context, plugin.Dependency, string, string) ([]byte, error)
}
type packageSkillSource struct {
	bundle    PackageSkillBundle
	authority PackageSkills
}

func (s *packageSkillSource) Lookup(ctx context.Context, name string) (sdk.Summary, error) {
	if err := skillSourceContextError(ctx, "lookup"); err != nil {
		return sdk.Summary{}, err
	}
	index := slices.IndexFunc(s.bundle.Skills, func(skill plugin.Skill) bool { return skill.Name == name })
	if index < 0 {
		return sdk.Summary{}, sdk.ErrSkillNotFound
	}
	skill := s.bundle.Skills[index]
	return sdk.Summary{Name: skill.Name, Description: skill.Description}, nil
}
func (s *packageSkillSource) document(ctx context.Context, name string) (*sdk.Skill, []byte, error) {
	if err := skillSourceContextError(ctx, "load"); err != nil {
		return nil, nil, err
	}
	content, err := s.authority.ReadSkillResource(ctx, s.bundle.Dependency, name, sdk.SkillFile)
	if cancelErr := skillSourceContextError(ctx, "load"); cancelErr != nil {
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
func (s *packageSkillSource) Load(ctx context.Context, name string) (*sdk.Skill, error) {
	skill, _, err := s.document(ctx, name)
	return skill, err
}
func (s *packageSkillSource) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	if err := skillSourceContextError(ctx, "open resource"); err != nil {
		return nil, err
	}
	content, err := s.authority.ReadSkillResource(ctx, s.bundle.Dependency, name, resource)
	if cancelErr := skillSourceContextError(ctx, "open resource"); cancelErr != nil {
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

// The returned file contains exactly the verified bytes; no pathname is reopened.
type skillResourceBytes struct {
	*bytes.Reader
	name string
}

func (f *skillResourceBytes) Close() error               { return nil }
func (f *skillResourceBytes) Stat() (fs.FileInfo, error) { return f, nil }
func (f *skillResourceBytes) Name() string               { return f.name }
func (f *skillResourceBytes) Size() int64                { return f.Reader.Size() }
func (f *skillResourceBytes) Mode() fs.FileMode          { return 0400 }
func (f *skillResourceBytes) ModTime() time.Time         { return time.Time{} }
func (f *skillResourceBytes) IsDir() bool                { return false }
func (f *skillResourceBytes) Sys() any                   { return nil }

func (s *packageSkillSource) List(ctx context.Context) ([]sdk.Summary, error) {
	if err := skillSourceContextError(ctx, "list"); err != nil {
		return nil, err
	}
	result := make([]sdk.Summary, 0, len(s.bundle.Skills))
	for _, skill := range s.bundle.Skills {
		result = append(result, sdk.Summary{Name: skill.Name, Description: skill.Description})
	}
	return result, nil
}

func (s *packageSkillSource) documentPath(name string) string {
	return filepath.Join(s.bundle.Root, name, sdk.SkillFile)
}
