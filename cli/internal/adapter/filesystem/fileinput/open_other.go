//go:build !unix

package fileinput

import "os"

func openPath(path string) (*os.File, error) {
	return os.Open(path)
}
