package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

const (
	maxWorkspaceCopyBytes     = 512 << 20
	maxWorkspaceCopyFileBytes = 128 << 20
	maxWorkspaceCopyEntries   = 100_000
	workspaceCopyBufferBytes  = 64 << 10
)

// copyTree materializes source directly into an empty destination. Both roots
// remain capability-confined for the whole copy; no archive-sized intermediate
// representation is created.
func copyTree(ctx context.Context, source, destination string) error {
	if ctx == nil {
		return errors.New("workspace copy context is required")
	}
	if !filepath.IsAbs(source) {
		return errors.New("workspace copy source must be absolute")
	}
	if !filepath.IsAbs(destination) {
		return errors.New("workspace copy destination must be absolute")
	}
	source = filepath.Clean(source)
	destination = filepath.Clean(destination)
	resolvedSource, err := resolveCopyRoot(source, "source")
	if err != nil {
		return err
	}
	resolvedDestination, err := resolveCopyRoot(destination, "destination")
	if err != nil {
		return err
	}
	if containsPath(resolvedSource.path, resolvedDestination.path) {
		return errors.New("workspace copy destination must not be inside source")
	}

	sourceRoot, sourceInfo, err := openCopyRoot(resolvedSource, "source")
	if err != nil {
		return err
	}
	defer func() { _ = sourceRoot.Close() }()
	destinationRoot, _, err := openCopyRoot(resolvedDestination, "destination")
	if err != nil {
		return err
	}
	defer func() { _ = destinationRoot.Close() }()

	copier := treeCopier{
		source:      sourceRoot,
		destination: destinationRoot,
		buffer:      make([]byte, workspaceCopyBufferBytes),
		maxEntries:  maxWorkspaceCopyEntries,
	}
	if err := copier.copyDirectory(ctx, ".", sourceInfo); err != nil {
		return err
	}
	return copier.restoreDirectoryModes(ctx)
}

type copyRoot struct {
	path string
	info os.FileInfo
}

func resolveCopyRoot(name, role string) (copyRoot, error) {
	physical, err := filepath.EvalSymlinks(name)
	if err != nil {
		return copyRoot{}, fmt.Errorf("resolve workspace copy %s: %w", role, err)
	}
	info, err := os.Stat(physical)
	if err != nil {
		return copyRoot{}, fmt.Errorf("stat workspace copy %s: %w", role, err)
	}
	if !info.IsDir() {
		return copyRoot{}, fmt.Errorf("workspace copy %s %q is not a directory", role, name)
	}
	return copyRoot{path: physical, info: info}, nil
}

func openCopyRoot(expected copyRoot, role string) (*os.Root, os.FileInfo, error) {
	root, err := os.OpenRoot(expected.path)
	if err != nil {
		return nil, nil, fmt.Errorf("open workspace copy %s: %w", role, err)
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, nil, fmt.Errorf("stat opened workspace copy %s: %w", role, err)
	}
	if !opened.IsDir() || !fileinput.SameVersion(expected.info, opened) {
		_ = root.Close()
		return nil, nil, fmt.Errorf("workspace copy %s changed while it was being opened", role)
	}
	return root, opened, nil
}

func containsPath(parent, candidate string) bool {
	relative, err := filepath.Rel(parent, candidate)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type directoryMode struct {
	name string
	mode fs.FileMode
}

type treeCopier struct {
	source      *os.Root
	destination *os.Root
	buffer      []byte
	directories []directoryMode
	maxEntries  int
	entries     int
	totalBytes  int64
}

func (t *treeCopier) copyDirectory(ctx context.Context, name string, expected fs.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, opened, err := t.readSourceDirectory(name, expected)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := t.copyEntry(ctx, filepath.Join(name, entry.Name()), entry); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return t.verifySourceDirectory(name, opened)
}

func (t *treeCopier) readSourceDirectory(
	name string,
	expected fs.FileInfo,
) (_ []os.DirEntry, _ os.FileInfo, err error) {
	remaining := t.maxEntries - t.entries
	if remaining < 0 {
		return nil, nil, fmt.Errorf("workspace has more than %d entries", t.maxEntries)
	}
	directory, opened, err := fileinput.OpenDirectoryAtExpected(t.source, name, expected)
	if err != nil {
		return nil, nil, fmt.Errorf("open source directory %q: %w", filepath.ToSlash(name), err)
	}
	defer func() {
		if closeErr := directory.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close source directory %q: %w", filepath.ToSlash(name), closeErr))
		}
	}()
	entries, readErr := directory.ReadDir(remaining + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	if readErr != nil {
		return nil, nil, fmt.Errorf("read source directory %q: %w", filepath.ToSlash(name), readErr)
	}
	if len(entries) > remaining {
		return nil, nil, fmt.Errorf("workspace has more than %d entries", t.maxEntries)
	}
	after, err := directory.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat source directory %q after reading: %w", filepath.ToSlash(name), err)
	}
	if !fileinput.SameVersion(opened, after) {
		return nil, nil, fmt.Errorf("source directory %q changed while it was being read", filepath.ToSlash(name))
	}
	t.entries += len(entries)
	slices.SortFunc(entries, func(left, right os.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})
	return entries, after, nil
}

func (t *treeCopier) copyEntry(ctx context.Context, name string, entry fs.DirEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := entry.Info()
	if err != nil {
		return fmt.Errorf("inspect source entry %q: %w", filepath.ToSlash(name), err)
	}
	portableName := filepath.ToSlash(name)
	localName := filepath.FromSlash(portableName)
	if parent := filepath.Dir(localName); parent != "." {
		if err := t.destination.MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("create parent for %q: %w", portableName, err)
		}
	}

	switch mode := info.Mode(); {
	case mode.IsDir():
		if err := t.destination.MkdirAll(localName, 0o700); err != nil {
			return fmt.Errorf("create directory %q: %w", portableName, err)
		}
		t.directories = append(t.directories, directoryMode{name: localName, mode: mode.Perm()})
		return t.copyDirectory(ctx, localName, info)
	case mode.IsRegular():
		return t.copyFile(ctx, portableName, localName, info)
	case mode&os.ModeSymlink != 0:
		return t.copySymlink(portableName, localName, info)
	default:
		return fmt.Errorf("unsupported file type %s at %q", mode.Type(), portableName)
	}
}

func (t *treeCopier) copyFile(
	ctx context.Context,
	portableName string,
	localName string,
	info fs.FileInfo,
) error {
	size := info.Size()
	if size < 0 || size > maxWorkspaceCopyFileBytes {
		return fmt.Errorf("file %q is %d bytes; limit is %d", portableName, size, maxWorkspaceCopyFileBytes)
	}
	if size > maxWorkspaceCopyBytes-t.totalBytes {
		return fmt.Errorf("workspace content exceeds %d bytes", maxWorkspaceCopyBytes)
	}
	t.totalBytes += size

	source, openedInfo, err := fileinput.OpenAtExpected(t.source, localName, info, maxWorkspaceCopyFileBytes)
	if err != nil {
		if errors.Is(err, fileinput.ErrChanged) || errors.Is(err, fileinput.ErrNotRegular) || errors.Is(err, fileinput.ErrTooLarge) {
			return fmt.Errorf("source file %q changed before copy: %w", portableName, err)
		}
		return fmt.Errorf("open source file %q: %w", portableName, err)
	}
	if openedInfo.Size() != size {
		closeErr := source.Close()
		return fmt.Errorf(
			"source file %q changed before copy: %w",
			portableName,
			errors.Join(errors.New("identity or size changed"), closeErr),
		)
	}
	destination, err := t.destination.OpenFile(
		localName,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		info.Mode().Perm(),
	)
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", portableName, errors.Join(err, source.Close()))
	}

	reader := io.LimitReader(contextReader{ctx: ctx, reader: source}, size+1)
	written, copyErr := io.CopyBuffer(writeOnly{writer: destination}, reader, t.buffer)
	after, sourceStatErr := source.Stat()
	current, pathStatErr := t.source.Lstat(localName)
	closeErr := errors.Join(destination.Close(), source.Close())
	if copyErr != nil || sourceStatErr != nil || pathStatErr != nil || closeErr != nil {
		return fmt.Errorf(
			"copy file %q: %w",
			portableName,
			errors.Join(copyErr, sourceStatErr, pathStatErr, closeErr),
		)
	}
	if written != size {
		return fmt.Errorf("source file %q changed during copy: copied %d bytes, expected %d", portableName, written, size)
	}
	if !fileinput.SameVersion(openedInfo, after) || !fileinput.SameVersion(after, current) {
		return fmt.Errorf("source file %q changed during copy", portableName)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (c contextReader) Read(buffer []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.reader.Read(buffer)
}

type writeOnly struct{ writer io.Writer }

func (w writeOnly) Write(buffer []byte) (int, error) {
	return w.writer.Write(buffer)
}

func (t *treeCopier) copySymlink(portableName, localName string, expected fs.FileInfo) error {
	before, err := t.source.Lstat(localName)
	if err != nil || !fileinput.SameVersion(expected, before) || before.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("source symlink %q changed before copy: %w", portableName, errors.Join(err, fileinput.ErrChanged))
	}
	target, err := t.source.Readlink(localName)
	if err != nil {
		return fmt.Errorf("read symlink %q: %w", portableName, err)
	}
	after, err := t.source.Lstat(localName)
	if err != nil || !fileinput.SameVersion(before, after) {
		return fmt.Errorf("source symlink %q changed during copy: %w", portableName, errors.Join(err, fileinput.ErrChanged))
	}
	portableTarget := filepath.ToSlash(target)
	if err := validateSymlinkTarget(portableName, portableTarget); err != nil {
		return err
	}
	if err := t.destination.Symlink(filepath.FromSlash(portableTarget), localName); err != nil {
		return fmt.Errorf("create symlink %q: %w", portableName, err)
	}
	return nil
}

func (t *treeCopier) verifySourceDirectory(name string, expected fs.FileInfo) error {
	current, err := t.source.Lstat(name)
	if err != nil {
		return fmt.Errorf("stat source directory %q after copy: %w", filepath.ToSlash(name), err)
	}
	if !fileinput.SameVersion(expected, current) {
		return fmt.Errorf("source directory %q changed during copy", filepath.ToSlash(name))
	}
	return nil
}

func (t *treeCopier) restoreDirectoryModes(ctx context.Context) error {
	slices.Reverse(t.directories)
	for _, directory := range t.directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.destination.Chmod(directory.name, directory.mode); err != nil {
			return fmt.Errorf("chmod directory %q: %w", directory.name, err)
		}
	}
	return nil
}

func validateSymlinkTarget(name, target string) error {
	const nullCharacter rune = 0

	platformTarget := filepath.FromSlash(target)
	if target == "" ||
		strings.ContainsRune(target, nullCharacter) ||
		strings.ContainsRune(target, '\\') ||
		isAbsolutePortablePath(target, platformTarget) {
		return fmt.Errorf("workspace symlink %q has unsafe target %q", name, target)
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("workspace symlink %q escapes via %q", name, target)
	}
	return nil
}

func isAbsolutePortablePath(portableName, platformName string) bool {
	if path.IsAbs(portableName) ||
		filepath.IsAbs(platformName) ||
		filepath.VolumeName(platformName) != "" {
		return true
	}
	return len(portableName) >= 2 &&
		((portableName[0] >= 'a' && portableName[0] <= 'z') ||
			(portableName[0] >= 'A' && portableName[0] <= 'Z')) &&
		portableName[1] == ':'
}
