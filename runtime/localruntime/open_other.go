//go:build !unix

package localruntime

import "os"

func openReadOnlyPath(path string) (*os.File, error) {
	return os.Open(path)
}
