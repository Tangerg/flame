package pluginpackage

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// staging is a private directory inside the release directory that receives
// one package's bytes. It owns the extraction limits, so neither a directory
// nor an archive source can place more than the package bounds on disk, and
// it is removed unless publication renames it into place.
type staging struct {
	path   string
	root   *os.Root
	copied int64
	// entries rejects a repeated name; folded rejects two names that would
	// select one destination on a case-insensitive or normalizing filesystem.
	entries map[string]bool
	folded  map[string]string
}

func newStaging(directory string) (*staging, error) {
	staged, err := os.MkdirTemp(directory, ".stage-")
	if err != nil {
		return nil, fmt.Errorf("pluginpackage: create staging directory: %w", err)
	}
	root, err := os.OpenRoot(staged)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("pluginpackage: open staging directory: %w", err), os.RemoveAll(staged))
	}
	return &staging{path: staged, root: root, entries: map[string]bool{}, folded: map[string]string{}}, nil
}

// discard releases the staging capability and removes staging that was not
// published. Sealed directories are made writable again so removal can finish.
func (s *staging) discard() error {
	return errors.Join(s.root.Close(), removeSealed(s.path))
}

// removeSealed removes a sealed tree, making its directories writable again so
// removal can finish. An absent tree is already removed.
func removeSealed(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	restoreErr := filepath.WalkDir(path, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(name, 0700)
		}
		return nil
	})
	return errors.Join(restoreErr, os.RemoveAll(path))
}

func (s *staging) extract(ctx context.Context, source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("pluginpackage: inspect package source: %w", err)
	}
	if info.IsDir() {
		return s.extractDirectory(ctx, source)
	}
	if info.Size() > MaxPackageBytes {
		return fmt.Errorf("%w: archive byte limit exceeded", plugin.ErrInvalid)
	}
	return s.extractArchive(ctx, source)
}

func (s *staging) extractDirectory(ctx context.Context, source string) (err error) {
	input, err := os.OpenRoot(source)
	if err != nil {
		return fmt.Errorf("pluginpackage: open package source: %w", err)
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	return s.copyDirectory(ctx, input)
}

func (s *staging) copyDirectory(ctx context.Context, input *os.Root) error {
	return fs.WalkDir(input.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if name == "." {
				return nil
			}
			return s.admitEntry(name, true)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: package must contain only directories and regular files", plugin.ErrInvalid)
		}
		f, err := input.Open(name)
		if err != nil {
			return err
		}
		return errors.Join(s.writeFile(ctx, name, info.Mode(), f), f.Close())
	})
}

func (s *staging) extractArchive(ctx context.Context, source string) (err error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		if errors.Is(err, zip.ErrFormat) {
			return fmt.Errorf("%w: package archive: %w", plugin.ErrInvalid, err)
		}
		return fmt.Errorf("pluginpackage: open package archive: %w", err)
	}
	defer func() {
		err = errors.Join(err, archive.Close())
		if errors.Is(err, zip.ErrFormat) || errors.Is(err, zip.ErrChecksum) || errors.Is(err, zip.ErrAlgorithm) {
			err = errors.Join(plugin.ErrInvalid, err)
		}
	}()
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			if entry.Mode()&fs.ModeType != fs.ModeDir {
				return fmt.Errorf("%w: invalid directory mode", plugin.ErrInvalid)
			}
			if err := s.admitEntry(strings.TrimSuffix(entry.Name, "/"), true); err != nil {
				return err
			}
			continue
		}
		f, err := entry.Open()
		if err != nil {
			return err
		}
		if err := errors.Join(s.writeFile(ctx, entry.Name, entry.Mode(), f), f.Close()); err != nil {
			return err
		}
	}
	return nil
}

func (s *staging) admitEntry(name string, isDir bool) error {
	if !plugin.ValidResourcePath(name) {
		return fmt.Errorf("%w: invalid package entry", plugin.ErrInvalid)
	}
	if s.entries[name] {
		return fmt.Errorf("%w: duplicate package entry", plugin.ErrInvalid)
	}
	s.entries[name] = true
	for segment := name; segment != "."; segment = path.Dir(segment) {
		key := strings.ToLower(norm.NFC.String(segment))
		if prior, found := s.folded[key]; found && prior != segment {
			return fmt.Errorf("%w: ambiguous package destination", plugin.ErrInvalid)
		}
		s.folded[key] = segment
	}
	if len(s.folded) > MaxFiles {
		return fmt.Errorf("%w: entry limit exceeded", plugin.ErrInvalid)
	}
	if isDir {
		return s.root.MkdirAll(name, 0700)
	}
	return nil
}

func (s *staging) writeFile(ctx context.Context, name string, mode fs.FileMode, input io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if mode&fs.ModeType != 0 {
		return fmt.Errorf("%w: invalid package entry mode", plugin.ErrInvalid)
	}
	if err := s.admitEntry(name, false); err != nil {
		return err
	}
	if err := s.root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	f, err := s.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600|mode.Perm()&0111)
	if err != nil {
		return err
	}
	limit := min(MaxFileBytes, MaxPackageBytes-s.copied)
	n, copyErr := io.Copy(f, io.LimitReader(input, limit+1))
	closeErr := f.Close()
	s.copied += n
	if n > limit {
		return errors.Join(fmt.Errorf("%w: byte limit exceeded", plugin.ErrInvalid), copyErr, closeErr)
	}
	return errors.Join(copyErr, closeErr)
}

// seal makes the staged tree read-only before it is digested, so the change
// stamps recorded with the digest are the ones the published release keeps.
// Only the execute bits a package declared survive.
func (s *staging) seal() error {
	return fs.WalkDir(s.root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mode := fs.FileMode(0400)
		if entry.IsDir() {
			mode = 0500
		} else {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			mode |= info.Mode().Perm() & 0111
		}
		return s.root.Chmod(name, mode)
	})
}
