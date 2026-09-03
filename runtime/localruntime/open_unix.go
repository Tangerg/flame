//go:build unix

package localruntime

import (
	"os"
	"syscall"
)

func openReadOnlyPath(path string) (*os.File, error) {
	// A special-node replacement must not suspend either credential validation
	// or the best-effort directory sync that follows publication.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
