// An open admits a path only when the opened descriptor still identifies the
// exact unchanged file observed before the open (SameVersion, not merely the
// same inode), so an in-place rewrite between stat and open is rejected here
// rather than deferred to the post-read VerifyPathVersion guard.
package filesystem

import (
	"errors"
	"fmt"
	"os"
)

var (
	ErrNotRegular   = errors.New("file input: source is not a regular file")
	ErrNotDirectory = errors.New("file input: source is not a directory")
	ErrFileTooLarge = errors.New("file input: source exceeds its byte limit")
	ErrChanged      = errors.New("file input: source changed")
)

type FileSizeLimitError struct {
	Size  int64
	Limit int64
}

var errRootRequired = errors.New("file input: root is required")

func (s FileSizeLimitError) Error() string {
	return fmt.Sprintf("file input: source uses %d bytes; limit is %d", s.Size, s.Limit)
}

func (s FileSizeLimitError) Unwrap() error { return ErrFileTooLarge }

func OpenFile(path string, maximumBytes int64) (*os.File, os.FileInfo, error) {
	return open(
		func() (os.FileInfo, error) { return os.Stat(path) },
		func() (*os.File, error) { return openPath(path) },
		maximumBytes,
	)
}

func OpenFileExpected(path string, expected os.FileInfo, maximumBytes int64) (*os.File, os.FileInfo, error) {
	return openExpected(expected, func() (*os.File, error) { return openPath(path) }, maximumBytes)
}

func OpenFileAt(root *os.Root, name string, maximumBytes int64) (*os.File, os.FileInfo, error) {
	if root == nil {
		return nil, nil, errRootRequired
	}
	return open(
		func() (os.FileInfo, error) { return root.Stat(name) },
		func() (*os.File, error) { return openRootPath(root, name) },
		maximumBytes,
	)
}

func OpenFileAtExpected(
	root *os.Root,
	name string,
	expected os.FileInfo,
	maximumBytes int64,
) (*os.File, os.FileInfo, error) {
	if root == nil {
		return nil, nil, errRootRequired
	}
	return openExpected(expected, func() (*os.File, error) { return openRootPath(root, name) }, maximumBytes)
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
	if !SameVersion(source, opened) {
		return nil, nil, ErrChanged
	}
	if err := validate(opened, maximumBytes); err != nil {
		return nil, nil, err
	}
	return file, opened, nil
}

func validate(info os.FileInfo, maximumBytes int64) error {
	if info == nil {
		return errors.New("file input: expected source information is required")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: mode %s", ErrNotRegular, info.Mode().Type())
	}
	if maximumBytes > 0 && info.Size() > maximumBytes {
		return FileSizeLimitError{Size: info.Size(), Limit: maximumBytes}
	}
	return nil
}

// Stricter than os.SameFile because content readers also depend on size, mode and
// modification time remaining stable.
func SameVersion(left, right os.FileInfo) bool {
	return left != nil && right != nil &&
		os.SameFile(left, right) &&
		left.Size() == right.Size() &&
		left.Mode() == right.Mode() &&
		left.ModTime().Equal(right.ModTime())
}

// Call it after consuming a complete value so an in-place mutation or path
// replacement cannot publish a torn snapshot.
func VerifyPathVersion(file *os.File, opened os.FileInfo, path string) error {
	return verifyVersion(file, opened, func() (os.FileInfo, error) {
		return os.Stat(path)
	})
}

func VerifyAtVersion(file *os.File, opened os.FileInfo, root *os.Root, name string) error {
	if root == nil {
		return errRootRequired
	}
	return verifyVersion(file, opened, func() (os.FileInfo, error) {
		return root.Stat(name)
	})
}

func verifyVersion(
	file *os.File,
	opened os.FileInfo,
	inspect func() (os.FileInfo, error),
) error {
	if file == nil {
		return errors.New("file input: opened file is required")
	}
	if opened == nil {
		return errors.New("file input: opened source information is required")
	}
	after, err := file.Stat()
	if err != nil {
		return fmt.Errorf("file input: inspect opened source after read: %w", err)
	}
	current, err := inspect()
	if err != nil {
		return fmt.Errorf("%w: inspect current source: %w", ErrChanged, err)
	}
	if !SameVersion(opened, after) || !SameVersion(after, current) {
		return ErrChanged
	}
	return nil
}
