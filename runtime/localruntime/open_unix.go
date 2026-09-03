//go:build unix

package localruntime

import (
	"os"
	"syscall"
)

func openTokenPath(path string) (*os.File, error) {
	// A special-node replacement must not suspend validation before the opened
	// identity and kind can be compared with the already-admitted token file.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
