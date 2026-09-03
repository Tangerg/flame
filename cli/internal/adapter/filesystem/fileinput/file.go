// Package fileinput opens verified CLI filesystem inputs without blocking on
// special nodes. It owns source identity, kind, and file-size admission;
// consumers retain content validation and user-facing error vocabulary.
package fileinput

import (
	"errors"
	"fmt"
	"os"
)

var (
	ErrNotRegular   = errors.New("file input: source is not a regular file")
	ErrNotDirectory = errors.New("file input: source is not a directory")
	ErrTooLarge     = errors.New("file input: source exceeds its byte limit")
	ErrChanged      = errors.New("file input: source changed while it was being opened")
)

// SizeLimitError carries the observed size and configured bound while
// retaining ErrTooLarge identity for consumer-specific presentation.
type SizeLimitError struct {
	Size  int64
	Limit int64
}

func (s SizeLimitError) Error() string {
	return fmt.Sprintf("file input: source uses %d bytes; limit is %d", s.Size, s.Limit)
}

func (s SizeLimitError) Unwrap() error { return ErrTooLarge }

// Open returns one read-only file only when the path still names the same
// bounded regular file observed before opening it. A zero limit is unbounded.
func Open(path string, maximumBytes int64) (*os.File, os.FileInfo, error) {
	return open(
		func() (os.FileInfo, error) { return os.Stat(path) },
		func() (*os.File, error) { return openPath(path) },
		maximumBytes,
	)
}

// OpenExpected opens path only when it still identifies expected.
func OpenExpected(path string, expected os.FileInfo, maximumBytes int64) (*os.File, os.FileInfo, error) {
	return openExpected(expected, func() (*os.File, error) { return openPath(path) }, maximumBytes)
}

// OpenAtExpected opens name beneath root only when it still identifies
// expected. Root confinement and non-blocking special-file admission apply to
// the same open.
func OpenAtExpected(root *os.Root, name string, expected os.FileInfo, maximumBytes int64) (*os.File, os.FileInfo, error) {
	if root == nil {
		return nil, nil, errors.New("file input: root is required")
	}
	return openExpected(expected, func() (*os.File, error) {
		return root.OpenFile(name, readOnlyFlags, 0)
	}, maximumBytes)
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
	return openExpected(source, openFile, maximumBytes)
}

func openExpected(
	source os.FileInfo,
	openFile func() (*os.File, error),
	maximumBytes int64,
) (_ *os.File, _ os.FileInfo, err error) {
	if maximumBytes < 0 {
		return nil, nil, errors.New("file input: byte limit must not be negative")
	}
	if err := validateFile(source, maximumBytes); err != nil {
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
	if err := validateFile(opened, maximumBytes); err != nil {
		return nil, nil, err
	}
	return file, opened, nil
}

func validateFile(info os.FileInfo, maximumBytes int64) error {
	if info == nil {
		return errors.New("file input: expected source information is required")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: mode %s", ErrNotRegular, info.Mode().Type())
	}
	if maximumBytes > 0 && info.Size() > maximumBytes {
		return SizeLimitError{Size: info.Size(), Limit: maximumBytes}
	}
	return nil
}
