package filesystem

import "errors"

var (
	ErrAbsolutePathRequired = errors.New("filesystem: absolute path required")
)
