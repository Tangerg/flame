// Package workspace contains focused project-scoped application use cases for
// workspace identity, browsing, knowledge, skills, hooks, and Git observation.
package workspace

import (
	"errors"
	"fmt"
	"reflect"
)

// Workspace input failures are stable application errors.
var (
	ErrCWDUnavailable       = errors.New("workspace: cwd unavailable")
	ErrPathRequired         = errors.New("workspace: path required")
	ErrPathOutsideRoot      = errors.New("workspace: path outside root")
	ErrInvalidFileRange     = errors.New("workspace: invalid file range")
	ErrFileReadTooLarge     = errors.New("workspace: file read exceeds its resource limit")
	ErrUnsupportedFile      = errors.New("workspace: file is not a supported UTF-8 text file")
	ErrGrepQueryMissing     = errors.New("workspace: grep query required")
	ErrInvalidGrepQuery     = errors.New("workspace: invalid grep query")
	ErrInvalidGrepLimit     = errors.New("workspace: invalid grep result limit")
	ErrGrepResultTooLarge   = errors.New("workspace: file search exceeds its resource limit")
	ErrInvalidFileReadLimit = errors.New("workspace: invalid file read byte limit")
)

// Paths resolves the externally-observed filesystem identity used by workspace
// use cases. Implementations own path canonicalization and symlink inspection;
// this package owns when each operation is required.
type Paths interface {
	ResolveExistingDir(path string) (string, error)
	ResolveInRoot(root, path string) (string, error)
	ResolveExistingInRoot(root, path string) (string, error)
}

// Scope resolves the workspace identity shared by independent use cases.
type Scope struct {
	defaultWorkspacePath string
	userHome             string
	paths                Paths
}

// NewScope constructs the shared workspace root scope.
func NewScope(defaultWorkspacePath, userHome string, paths Paths) (*Scope, error) {
	value := reflect.ValueOf(paths)
	if !value.IsValid() {
		return nil, errors.New("workspace: path resolver is required")
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, errors.New("workspace: path resolver is required")
		}
	}
	return &Scope{defaultWorkspacePath: defaultWorkspacePath, userHome: userHome, paths: paths}, nil
}

func (s *Scope) root(cwd string) (string, error) {
	root := cwd
	if root == "" {
		root = s.defaultWorkspacePath
	}
	resolved, err := s.paths.ResolveExistingDir(root)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrCWDUnavailable, root, err)
	}
	return resolved, nil
}

// ResolveRoot returns the effective, existing working directory for a workspace
// request. Empty cwd selects the host-provided default workspace.
func (s *Scope) ResolveRoot(cwd string) (string, error) {
	return s.root(cwd)
}
