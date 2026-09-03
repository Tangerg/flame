package fileinput

import (
	"errors"
	"os"
)

// OpenDirectory opens path only when it remains the same directory observed
// before the non-blocking open.
func OpenDirectory(path string) (_ *os.File, _ os.FileInfo, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, ErrNotDirectory
	}
	directory, err := openPath(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, directory.Close())
		}
	}()
	opened, err := directory.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !opened.IsDir() || !os.SameFile(info, opened) {
		return nil, nil, ErrChanged
	}
	return directory, opened, nil
}
