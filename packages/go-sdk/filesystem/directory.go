package filesystem

import (
	"errors"
	"os"
)

func OpenDirectory(path string) (*os.File, os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	return OpenDirectoryExpected(path, info)
}

func OpenDirectoryExpected(path string, expected os.FileInfo) (*os.File, os.FileInfo, error) {
	return openExpectedDirectory(expected, func() (*os.File, error) { return openPath(path) })
}

func OpenDirectoryAt(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	if root == nil {
		return nil, nil, errRootRequired
	}
	info, err := root.Stat(name)
	if err != nil {
		return nil, nil, err
	}
	return openExpectedDirectory(info, func() (*os.File, error) { return openRootPath(root, name) })
}

func OpenDirectoryAtExpected(root *os.Root, name string, expected os.FileInfo) (*os.File, os.FileInfo, error) {
	if root == nil {
		return nil, nil, errRootRequired
	}
	return openExpectedDirectory(expected, func() (*os.File, error) { return openRootPath(root, name) })
}

func openExpectedDirectory(
	source os.FileInfo,
	openDirectory func() (*os.File, error),
) (_ *os.File, _ os.FileInfo, err error) {
	if source == nil {
		return nil, nil, errors.New("file input: expected source information is required")
	}
	if !source.IsDir() {
		return nil, nil, ErrNotDirectory
	}
	directory, err := openDirectory()
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
	if !opened.IsDir() || !os.SameFile(source, opened) {
		return nil, nil, ErrChanged
	}
	return directory, opened, nil
}
