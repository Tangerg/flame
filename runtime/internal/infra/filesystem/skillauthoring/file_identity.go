package skillauthoring

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

func moveDirectoryID(root *os.Root, path string) (string, error) {
	info, err := root.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %q is not a directory", skills.ErrConflict, path)
	}
	directory, _, err := filesystem.OpenDirectoryAtExpected(root, path, info)
	if err != nil {
		return "", err
	}
	return closeWithFileID(directory)
}

func documentFileID(root *os.Root, path string) (string, error) {
	info, err := root.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	file, _, err := filesystem.OpenFileAtExpected(root, path, info, 0)
	if err != nil {
		return "", err
	}
	return closeWithFileID(file)
}

func closeWithFileID(file *os.File) (string, error) {
	device, inode, err := fileIdentity(file)
	if err := errors.Join(err, file.Close()); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x:%x", device, inode), nil
}
