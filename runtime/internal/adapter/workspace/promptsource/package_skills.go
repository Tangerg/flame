package promptsource

import (
	"bytes"
	"context"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skilldocument"
	sdk "github.com/Tangerg/scope/skills"
)

type PackageSkillBundle struct {
	Root       string
	Dependency plugin.Dependency
	Names      []string
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
	skill, err := s.Load(ctx, name)
	if err != nil {
		return sdk.Summary{}, err
	}
	return skill.Summary(), nil
}
func (s *packageSkillSource) document(ctx context.Context, name string) (*sdk.Skill, []byte, error) {
	content, err := s.authority.ReadSkillResource(ctx, s.bundle.Dependency, name, sdk.SkillFile)
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
	content, err := s.authority.ReadSkillResource(ctx, s.bundle.Dependency, name, resource)
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
	result := make([]sdk.Summary, 0, len(s.bundle.Names))
	for _, name := range s.bundle.Names {
		value, err := s.Lookup(ctx, name)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (s *packageSkillSource) documentPath(name string) string {
	return filepath.Join(s.bundle.Root, name, sdk.SkillFile)
}
