//go:build !unix

package localruntime

import "os"

const readOnlyFlags = os.O_RDONLY

func openReadOnlyPath(path string) (*os.File, error) {
	return os.OpenFile(path, readOnlyFlags, 0)
}
