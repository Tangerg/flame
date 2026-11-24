package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/repository"
)

type Resolver struct{ filesystem.DirectoryResolver }

func (Resolver) ResolveInRoot(root, path string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", filesystem.ErrAbsolutePathRequired
	}
	if path == "" {
		return "", workspaceapp.ErrPathRequired
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, path)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return rel, nil
}

func (r Resolver) ResolveExistingInRoot(root, path string) (string, error) {
	rel, err := r.ResolveInRoot(root, path)
	if err != nil {
		return "", err
	}
	physicalRoot, err := filesystem.ResolvePath("", root)
	if err != nil {
		return "", err
	}
	physicalTarget, err := filesystem.ResolvePath(physicalRoot, rel)
	if err != nil {
		return "", err
	}
	inside, err := filesystem.ContainsPath(physicalRoot, physicalTarget)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return rel, nil
}

func (Resolver) Inspect(path string) (workspaceapp.Resolved, error) {
	cwd, err := filesystem.CanonicalPath(path)
	if err != nil {
		return workspaceapp.Resolved{}, err
	}
	identity := workspaceapp.Resolved{Path: cwd, RepositoryRoot: cwd}
	info, err := os.Stat(cwd)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			identity.Missing = true
			return identity, nil
		}
		return workspaceapp.Resolved{}, err
	}
	if !info.IsDir() {
		identity.Missing = true
		return identity, nil
	}

	root, err := repository.Root(cwd)
	if err != nil {
		return workspaceapp.Resolved{}, err
	}
	identity.RepositoryRoot = root
	return identity, nil
}
