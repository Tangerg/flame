//go:build unix

package fileinput

import (
	"os"

	"golang.org/x/sys/unix"
)

const readOnlyFlags = os.O_RDONLY | unix.O_NONBLOCK

func openPath(path string) (*os.File, error) {
	return os.OpenFile(path, readOnlyFlags, 0)
}

func openRootPath(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, readOnlyFlags, 0)
}
