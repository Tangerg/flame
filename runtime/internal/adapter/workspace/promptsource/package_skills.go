package promptsource

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
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

var errPackageSkillConflict = errors.New("promptsource: package Skill name conflict")

func (l runtimeSkillLayers) packageSource(name string) (*packageSkillSource, error) {
	var selected *packageSkillSource
	for index := range l.packages {
		candidate := &l.packages[index]
		if !slices.Contains(candidate.bundle.Names, name) {
			continue
		}
		if selected != nil {
			return nil, errors.Join(workspaceapp.ErrSkillUnavailable, errPackageSkillConflict)
		}
		selected = candidate
	}
	if selected == nil {
		return nil, sdk.ErrSkillNotFound
	}
	return selected, nil
}
func (l runtimeSkillLayers) modelSource(ctx context.Context, name string, decorateUser func(sdk.ResourceSource) sdk.ResourceSource) (sdk.ResourceSource, error) {
	for _, source := range []*runtimeSkillSource{l.project, l.user} {
		if source == nil {
			continue
		}
		entries, err := source.directoryEntries(ctx)
		if err != nil {
			return nil, err
		}
		if slices.Contains(skillCandidateNames(entries), name) {
			if source == l.user && decorateUser != nil {
				return decorateUser(source), nil
			}
			return source, nil
		}
	}
	selected, err := l.packageSource(name)
	if err != nil {
		return nil, err
	}
	return selected, nil
}
func (s *runtimeSkillOverlay) Lookup(ctx context.Context, name string) (sdk.Summary, error) {
	source, err := s.layers.modelSource(ctx, name, s.decorateUser)
	if err != nil {
		return sdk.Summary{}, err
	}
	return source.Lookup(ctx, name)
}
func (s *runtimeSkillOverlay) Load(ctx context.Context, name string) (*sdk.Skill, error) {
	source, err := s.layers.modelSource(ctx, name, s.decorateUser)
	if err != nil {
		return nil, err
	}
	return source.Load(ctx, name)
}
func (s *runtimeSkillOverlay) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	source, err := s.layers.modelSource(ctx, name, s.decorateUser)
	if err != nil {
		return nil, err
	}
	return source.OpenResource(ctx, name, resource)
}
func (l runtimeSkillLayers) dependencies(ctx context.Context) ([]plugin.Dependency, error) {
	catalog, err := l.list(ctx)
	if err != nil {
		return nil, err
	}
	var result []plugin.Dependency
	for _, skill := range catalog.Skills {
		dependency, found := skill.Source.Installation()
		if !found {
			continue
		}
		if !slices.Contains(result, dependency) {
			result = append(result, dependency)
		}
	}
	return result, nil
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
	skill, err := LoadSkillDocument(ctx, name, content)
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
