package workspace

import "errors"

var (
	ErrFileListTooLarge    = errors.New("workspace: file listing too large")
	ErrInvalidFileListPath = errors.New("workspace: file listing path is not a directory")
	ErrInvalidFileGlob     = errors.New("workspace: invalid file glob")
	ErrPageLimit           = errors.New("workspace: page limit invalid")
	ErrPageCursor          = errors.New("workspace: page cursor invalid")
	ErrVCSUnavailable      = errors.New("workspace: VCS unavailable")
	ErrVCSBaseUnknown      = errors.New("workspace: VCS base unknown")
	ErrVCSResultTooLarge   = errors.New("workspace: VCS result too large")
)
