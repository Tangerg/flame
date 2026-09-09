//go:build unix

package procgroup

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Prepare gives command its own process group so Stop can reach what it spawns.
func Prepare(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Stop kills command's whole process group. The negative pid is the point: a
// child that launched its own children — a package runner, a language server, a
// shell pipeline — leaves them running when only the direct child is signalled.
func Stop(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
