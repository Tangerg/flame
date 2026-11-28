package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/repository"
)

type Resolver struct{}

func (Resolver) CanonicalPath(path string) (string, error) {
	resolved, err := filesystem.CanonicalPath(path)
	if err != nil {
		return "", unavailableWorkspace(path, err)
	}
	return resolved, nil
}

func (Resolver) ResolveExistingDir(path string) (string, error) {
	resolved, err := (filesystem.DirectoryResolver{}).ResolveExistingDir(path)
	if err != nil {
		return "", unavailableWorkspace(path, err)
	}
	return resolved, nil
}

func unavailableWorkspace(path string, err error) error {
	return fmt.Errorf("%w: %q: %w", workspaceapp.ErrCWDUnavailable, path, err)
}

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
	rel, inside := relativeInTree(root, abs)
	if !inside {
		return "", workspaceapp.ErrPathOutsideRoot
	}
	return rel, nil
}

func relativeInTree(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (r Resolver) RemovalAffectsDirectory(root, address string) (bool, error) {
	// The saved address can be lost even if it currently points outside the tree.
	if _, inside := relativeInTree(root, address); inside {
		return true, nil
	}
	current, err := r.ResolveExistingDir(address)
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, syscall.ENOTDIR), errors.Is(err, filesystem.ErrNotDirectory):
		return false, nil
	case err != nil:
		return false, err
	}
	_, inside := relativeInTree(root, current)
	return inside, nil
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

func (r Resolver) Inspect(path string) (workspaceapp.DirectoryInspection, error) {
	info, err := r.Resolve(path)
	return info.DirectoryInspection, err
}

func (r Resolver) Resolve(path string) (workspaceapp.Info, error) {
	cwd, err := r.CanonicalPath(path)
	if err != nil {
		return workspaceapp.Info{}, err
	}
	workspace := workspaceapp.Info{Path: cwd, DirectoryInspection: workspaceapp.DirectoryInspection{RepositoryRoot: cwd}}
	info, err := os.Stat(cwd)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			workspace.Missing = true
			return workspace, nil
		}
		return workspaceapp.Info{}, unavailableWorkspace(path, err)
	}
	if !info.IsDir() {
		workspace.Missing = true
		return workspace, nil
	}

	root, err := repository.Root(cwd)
	if err != nil {
		return workspaceapp.Info{}, err
	}
	workspace.RepositoryRoot = root
	return workspace, nil
}
