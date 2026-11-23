package session

import (
	"fmt"
	pathpkg "path"
)

type Workspace struct {
	path string
}

func NewWorkspace(path string) (Workspace, error) {
	if path == "" {
		return Workspace{}, fmt.Errorf("%w: workspace is required", ErrInvalid)
	}
	if !pathpkg.IsAbs(path) {
		return Workspace{}, fmt.Errorf("%w: workspace path %q is not absolute", ErrInvalid, path)
	}
	if clean := pathpkg.Clean(path); clean != path {
		return Workspace{}, fmt.Errorf("%w: workspace path %q is not canonical (want %q)", ErrInvalid, path, clean)
	}
	return Workspace{path: path}, nil
}

func (w Workspace) Path() string { return w.path }

func (w Workspace) Validate() error {
	if w.path == "" {
		return fmt.Errorf("%w: workspace is required", ErrInvalid)
	}
	return nil
}
