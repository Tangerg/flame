// Package skilldocument binds verified document bytes to Scope's Skill loader.
package skilldocument

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	sdk "github.com/Tangerg/scope/skills"
)

// Load lets Scope own format and directory identity without reopening a path
// whose content may have changed since the caller verified it.
func Load(ctx context.Context, name string, content []byte) (*sdk.Skill, error) {
	if len(content) > skills.MaxAuthoredSkillDocumentBytes {
		return nil, fmt.Errorf("%w: %q exceeds %d bytes", skills.ErrDocumentTooLarge, name, skills.MaxAuthoredSkillDocumentBytes)
	}
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("%w %q: document is not UTF-8", sdk.ErrInvalidSkill, name)
	}
	repository, err := sdk.NewRepository(documentFS{name: name, content: content}, sdk.RepositoryConfig{MaxSkillBytes: skills.MaxAuthoredSkillDocumentBytes})
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
	if name != f.name+"/"+sdk.SkillFile {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &documentFile{bytes.NewReader(f.content)}, nil
}

type documentFile struct{ *bytes.Reader }

func (f *documentFile) Close() error               { return nil }
func (f *documentFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *documentFile) Name() string               { return sdk.SkillFile }
func (f *documentFile) Size() int64                { return f.Reader.Size() }
func (f *documentFile) Mode() fs.FileMode          { return 0400 }
func (f *documentFile) ModTime() time.Time         { return time.Time{} }
func (f *documentFile) IsDir() bool                { return false }
func (f *documentFile) Sys() any                   { return nil }
