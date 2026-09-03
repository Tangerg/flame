//go:build !unix

package localruntime

import "os"

func openTokenPath(path string) (*os.File, error) {
	return os.Open(path)
}
