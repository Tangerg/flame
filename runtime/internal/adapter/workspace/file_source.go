package workspace

import (
	"errors"
	"fmt"
	"os"
)

var (
	errFileSourceNotRegular = errors.New("workspace: file source is not regular")
	errFileSourceTooLarge   = errors.New("workspace: file source is too large")
)

// openRegularFile validates before os.Open so a workspace FIFO or device
// cannot block a request before context-aware reading begins. The second
// inspection rejects a path replaced during the open boundary.
func openRegularFile(path string, maximumBytes int64) (_ *os.File, err error) {
	source, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if err := validateRegularFile(source, maximumBytes); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(source, opened) {
		return nil, errors.New("workspace: file source changed while it was being opened")
	}
	if err := validateRegularFile(opened, maximumBytes); err != nil {
		return nil, err
	}
	return file, nil
}

func validateRegularFile(info os.FileInfo, maximumBytes int64) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: mode %s", errFileSourceNotRegular, info.Mode().Type())
	}
	if info.Size() > maximumBytes {
		return fmt.Errorf("%w: uses %d bytes", errFileSourceTooLarge, info.Size())
	}
	return nil
}
