package filesystem

import (
	"os"
	"path/filepath"
)

type DirectoryResolver struct{}

// An unresolved physical identity cannot become a competing lexical identity.
// Relative values are rejected because they depend on the process cwd.
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
