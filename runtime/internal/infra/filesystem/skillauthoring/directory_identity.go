package skillauthoring

import (
	"errors"
	"fmt"
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
	device, inode, err := directoryIdentity(directory)
	if err := errors.Join(err, directory.Close()); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x:%x", device, inode), nil
}
