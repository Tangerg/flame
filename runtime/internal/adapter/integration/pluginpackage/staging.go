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

	"github.com/Tangerg/flame/runtime/internal/cancellation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

// staging is a private directory inside the release directory that receives
// one package's bytes. It owns the extraction limits, so neither a directory
// nor an archive source can place more than the package bounds on disk, and
// it is removed unless publication renames it into place.
type staging struct {
	path   string
	root   *os.Root
	copied int64
	// entries owns the complete destination tree, including implied parents.
	// Keys identify one destination on case-insensitive or normalizing filesystems.
	entries map[string]packageEntry
}

type entryKind uint8

const (
	implicitDirectory entryKind = iota + 1
	directoryEntry
	fileEntry
)

type packageEntry struct {
	name string
	kind entryKind
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
	return &staging{path: staged, root: root, entries: map[string]packageEntry{}}, nil
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
	if cause := cancellation.Err(ctx); cause != nil {
		return cause
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("pluginpackage: inspect package source: %w", err)
	}
	if info.IsDir() {
		return s.extractDirectory(ctx, source)
	}
	return s.extractArchive(ctx, source, info)
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
		if cause := cancellation.Err(ctx); cause != nil {
			return cause
		}
		if entry.IsDir() {
			if name == "." {
				return nil
			}
			return s.admitEntry(name, directoryEntry)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		f, opened, err := fileinput.OpenAtExpected(input, name, info, MaxFileBytes)
		if err != nil {
			return packageInputError(err)
		}
		copyErr := s.writeFile(ctx, name, opened.Mode(), f)
		verifyErr := fileinput.VerifyAtVersion(f, opened, input, name)
		return packageInputError(errors.Join(copyErr, verifyErr, f.Close()))
	})
}

func (s *staging) extractArchive(ctx context.Context, source string, expected fs.FileInfo) (err error) {
	file, opened, err := fileinput.OpenExpected(source, expected, MaxPackageBytes)
	if err != nil {
		return fmt.Errorf("pluginpackage: open package archive: %w", packageInputError(err))
	}
	defer func() {
		err = packageInputError(errors.Join(err, fileinput.VerifyPathVersion(file, opened, source), file.Close()))
		if errors.Is(err, zip.ErrFormat) || errors.Is(err, zip.ErrChecksum) || errors.Is(err, zip.ErrAlgorithm) {
			err = errors.Join(plugin.ErrInvalid, err)
		}
	}()
	archive, err := zip.NewReader(file, opened.Size())
	if err != nil {
		return fmt.Errorf("pluginpackage: package archive: %w", err)
	}
	for _, entry := range archive.File {
		if cause := cancellation.Err(ctx); cause != nil {
			return cause
		}
		if entry.FileInfo().IsDir() {
			if entry.Mode()&fs.ModeType != fs.ModeDir {
				return fmt.Errorf("%w: invalid directory mode", plugin.ErrInvalid)
			}
			if err := s.admitEntry(strings.TrimSuffix(entry.Name, "/"), directoryEntry); err != nil {
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

func packageInputError(err error) error {
	if errors.Is(err, fileinput.ErrNotRegular) || errors.Is(err, fileinput.ErrTooLarge) || errors.Is(err, fileinput.ErrChanged) {
		return errors.Join(plugin.ErrInvalid, err)
	}
	return err
}

func (s *staging) admitEntry(name string, kind entryKind) error {
	if !plugin.ValidResourcePath(name) {
		return fmt.Errorf("%w: invalid package entry", plugin.ErrInvalid)
	}
	var added int
	for segment := name; segment != "."; segment = path.Dir(segment) {
		prior, found := s.entries[destinationKey(segment)]
		if !found {
			added++
			continue
		}
		if prior.name != segment {
			return fmt.Errorf("%w: ambiguous package destination", plugin.ErrInvalid)
		}
		if segment == name {
			if prior.kind != implicitDirectory || kind != directoryEntry {
				return fmt.Errorf("%w: duplicate or conflicting package entry", plugin.ErrInvalid)
			}
		} else if prior.kind == fileEntry {
			return fmt.Errorf("%w: package entry parent is a file", plugin.ErrInvalid)
		}
	}
	if len(s.entries)+added > MaxFiles {
		return fmt.Errorf("%w: entry limit exceeded", plugin.ErrInvalid)
	}
	for segment := name; segment != "."; segment = path.Dir(segment) {
		key := destinationKey(segment)
		if segment == name {
			s.entries[key] = packageEntry{name: segment, kind: kind}
		} else if _, found := s.entries[key]; !found {
			s.entries[key] = packageEntry{name: segment, kind: implicitDirectory}
		}
	}
	if kind == directoryEntry {
		return s.root.MkdirAll(name, 0700)
	}
	return nil
}

func destinationKey(name string) string {
	return strings.ToLower(norm.NFC.String(name))
}

func (s *staging) writeFile(ctx context.Context, name string, mode fs.FileMode, input io.Reader) error {
	if cause := cancellation.Err(ctx); cause != nil {
		return cause
	}
	if mode&fs.ModeType != 0 {
		return fmt.Errorf("%w: invalid package entry mode", plugin.ErrInvalid)
	}
	if err := s.admitEntry(name, fileEntry); err != nil {
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
	n, copyErr := io.Copy(f, io.LimitReader(cancellation.Reader(ctx, input), limit+1))
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
