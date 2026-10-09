//go:build unix && !darwin

package procgroup

import (
	"errors"
	"os"
	"syscall"
)

func stopGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
