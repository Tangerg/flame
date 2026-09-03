//go:build unix

package fileinput

import (
	"os"
	"syscall"
)

const readOnlyFlags = os.O_RDONLY | syscall.O_NONBLOCK

func openPath(path string) (*os.File, error) {
	return os.OpenFile(path, readOnlyFlags, 0)
}
