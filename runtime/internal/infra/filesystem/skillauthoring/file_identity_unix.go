//go:build unix

package skillauthoring

import (
	"os"

	"golang.org/x/sys/unix"
)

func fileIdentity(file *os.File) (device, inode uint64, err error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return 0, 0, err
	}
	return uint64(stat.Dev), uint64(stat.Ino), nil
}
