package skilldocument

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	skillspec "github.com/Tangerg/scope/skills"
)

const openOperation = "open"

func Load(ctx context.Context, name string, content []byte) (*skillspec.Skill, error) {
	if len(content) > skills.MaxAuthoredSkillDocumentBytes {
		return nil, fmt.Errorf("%w: %q exceeds %d bytes", skills.ErrDocumentTooLarge, name, skills.MaxAuthoredSkillDocumentBytes)
	}
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("%w %q: document is not UTF-8", skillspec.ErrInvalidSkill, name)
	}
	repository, err := skillspec.NewRepository(documentFS{name: name, content: content}, skillspec.RepositoryConfig{MaxSkillBytes: skills.MaxAuthoredSkillDocumentBytes})
	if err != nil {
		return nil, err
	}
	return repository.Load(ctx, name)
}

type documentFS struct {
	name    string
	content []byte
}

func (f documentFS) Open(name string) (fs.File, error) {
	if name != f.name+"/"+skillspec.SkillFile {
		return nil, &fs.PathError{Op: openOperation, Path: name, Err: fs.ErrNotExist}
	}
	return &documentFile{bytes.NewReader(f.content)}, nil
}

func (f documentFS) OpenInDir(dir, name string) (fs.File, error) {
	if !fs.ValidPath(dir) || !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: openOperation, Path: name, Err: fs.ErrInvalid}
	}
	return f.Open(path.Join(dir, name))
}

type documentFile struct{ *bytes.Reader }

func (f *documentFile) Close() error               { return nil }
func (f *documentFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *documentFile) Name() string               { return skillspec.SkillFile }
func (f *documentFile) Size() int64                { return f.Reader.Size() }
func (f *documentFile) Mode() fs.FileMode          { return filesystem.ReadOnlyFileMode }
func (f *documentFile) ModTime() time.Time         { return time.Time{} }
func (f *documentFile) IsDir() bool                { return false }
func (f *documentFile) Sys() any                   { return nil }
