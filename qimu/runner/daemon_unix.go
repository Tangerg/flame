//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package runner

import (
	"os/exec"
	"syscall"
)

func detachDaemon(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return nil
}
