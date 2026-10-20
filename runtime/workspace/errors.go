package workspace

import "errors"

var (
	ErrAbsolutePathRequired = errors.New("workspace: absolute path required")
	ErrNotDirectory         = errors.New("workspace: not a directory")
	ErrCWDUnavailable       = errors.New("workspace: cwd unavailable")
)
