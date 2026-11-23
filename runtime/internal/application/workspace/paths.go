package workspace

import (
	"errors"
	"fmt"

	"github.com/samber/lo"
)

var (
	ErrCWDUnavailable       = errors.New("workspace: cwd is unavailable")
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

type paths interface {
	ResolveExistingDir(path string) (string, error)
	ResolveInRoot(root, path string) (string, error)
	ResolveExistingInRoot(root, path string) (string, error)
}

type Scope struct {
	defaultWorkspacePath string
	userHome             string
	paths                paths
}

func NewScope(defaultWorkspacePath, userHome string, paths paths) (*Scope, error) {
	if lo.IsNil(paths) {
		return nil, errors.New("workspace: path resolver is required")
	}
	return &Scope{defaultWorkspacePath: defaultWorkspacePath, userHome: userHome, paths: paths}, nil
}

func (s *Scope) ResolveRoot(cwd string) (string, error) {
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
