//go:build !unix

package fileinput

import "os"

func openPath(path string) (*os.File, error) {
	return os.Open(path)
}

func openRootPath(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}
