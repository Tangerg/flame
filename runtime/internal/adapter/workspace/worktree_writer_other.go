//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package workspace

import (
	"os"
	"os/exec"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

func retainTaskWriterDirectory(*exec.Cmd, string) (*os.File, error) {
	return nil, filesystem.ErrLockUnsupported
}

func inheritedTaskWriterDirectory() *os.File { return nil }
