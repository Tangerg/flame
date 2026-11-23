package filesystem

import (
	"os"
	"path/filepath"
)

type DirectoryResolver struct{}

func CanonicalPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", ErrAbsolutePathRequired
	}
	return ResolvePath("", path)
}

func (DirectoryResolver) ResolveExistingDir(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", ErrAbsolutePathRequired
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrNotDirectory
	}
	return CanonicalPath(path)
}
