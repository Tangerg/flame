// Package fileinput opens bounded regular files without blocking on special
// filesystem nodes. It owns source identity and size admission; callers retain
// content validation and translation into application errors.
package fileinput

import (
	"errors"
	"fmt"
	"os"
)

var (
	ErrNotRegular = errors.New("file input: source is not a regular file")
	ErrTooLarge   = errors.New("file input: source exceeds its byte limit")
	ErrChanged    = errors.New("file input: source changed while it was being opened")
)

// Open returns one read-only file only when the path still names the same
// bounded regular file observed before opening it. A zero limit is unbounded.
func Open(path string, maximumBytes int64) (*os.File, os.FileInfo, error) {
	return open(
		func() (os.FileInfo, error) { return os.Stat(path) },
		func() (*os.File, error) { return openPath(path) },
		maximumBytes,
	)
}

// OpenAt applies Open's admission policy through an os.Root, retaining the
// root's traversal and symlink confinement.
func OpenAt(root *os.Root, name string, maximumBytes int64) (*os.File, os.FileInfo, error) {
	if root == nil {
		return nil, nil, errors.New("file input: root is required")
	}
	return open(
		func() (os.FileInfo, error) { return root.Stat(name) },
		func() (*os.File, error) { return openRootPath(root, name) },
		maximumBytes,
	)
}

func open(
	inspect func() (os.FileInfo, error),
	openFile func() (*os.File, error),
	maximumBytes int64,
) (_ *os.File, _ os.FileInfo, err error) {
	if maximumBytes < 0 {
		return nil, nil, errors.New("file input: byte limit must not be negative")
	}
	source, err := inspect()
	if err != nil {
		return nil, nil, err
	}
	if err := validate(source, maximumBytes); err != nil {
		return nil, nil, err
	}
	file, err := openFile()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()
	opened, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !os.SameFile(source, opened) {
		return nil, nil, ErrChanged
	}
	if err := validate(opened, maximumBytes); err != nil {
		return nil, nil, err
	}
	return file, opened, nil
}

func validate(info os.FileInfo, maximumBytes int64) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: mode %s", ErrNotRegular, info.Mode().Type())
	}
	if maximumBytes > 0 && info.Size() > maximumBytes {
		return fmt.Errorf("%w: source uses %d bytes; limit is %d", ErrTooLarge, info.Size(), maximumBytes)
	}
	return nil
}
