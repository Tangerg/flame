//go:build !unix

package procgroup

import (
	"os"
	"os/exec"
)

func Prepare(*exec.Cmd) {}

func Stop(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
