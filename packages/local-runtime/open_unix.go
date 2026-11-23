//go:build unix

package localruntime

import (
	"os"
	"syscall"
)

const readOnlyFlags = os.O_RDONLY | syscall.O_NONBLOCK

func openReadOnlyPath(path string) (*os.File, error) {
	return os.OpenFile(path, readOnlyFlags, 0)
}
