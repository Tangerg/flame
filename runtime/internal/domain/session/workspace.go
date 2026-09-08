package session

import (
	"fmt"
	pathpkg "path"
)

// Workspace is the exact canonical filesystem identity admitted for a
// Session. It owns only pure path invariants; filesystem existence, symlink
// resolution, and physical canonicalization are outside this value.
type Workspace struct {
	path string
}

// NewWorkspace constructs an exact workspace identity from an already
// admitted path. The path must be absolute and lexically canonical.
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

// Path returns the exact canonical path.
func (w Workspace) Path() string { return w.path }

// Validate verifies that w was constructed. Absoluteness and canonical
// spelling are established by NewWorkspace, the only source of the path.
func (w Workspace) Validate() error {
	if w.path == "" {
		return fmt.Errorf("%w: workspace is required", ErrInvalid)
	}
	return nil
}
