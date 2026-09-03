//go:build !unix

package fileinput

import "os"

const readOnlyFlags = os.O_RDONLY

func openPath(path string) (*os.File, error) {
	return os.Open(path)
}
