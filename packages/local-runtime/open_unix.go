//go:build unix

package localruntime

import (
	"os"
	"syscall"
)

const readOnlyFlags = os.O_RDONLY | syscall.O_NONBLOCK

func openReadOnlyPath(path string) (*os.File, error) {
	// A special-node replacement must not suspend either credential validation
	// or the best-effort directory sync that follows publication.
	return os.OpenFile(path, readOnlyFlags, 0)
}
