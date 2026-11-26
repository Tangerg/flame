//go:build !unix && !windows

package skillauthoring

import (
	"errors"
	"os"
)

func directoryIdentity(*os.File) (device, inode uint64, err error) {
	return 0, 0, errors.New("skillauthoring: stable directory identity is unsupported on this platform")
}
