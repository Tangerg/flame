//go:build unix

package procgroup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func Prepare(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func Stop(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	err := stopGroup(command.Process.Pid)
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("procgroup: stop group %d: %w", command.Process.Pid, err)
	}
	return err
}
