//go:build !unix

package procgroup

import (
	"os"
	"os/exec"
)

// Prepare is a no-op where the OS has no process groups to join.
func Prepare(*exec.Cmd) {}

// Stop ends the process itself, which is as far as this platform reaches.
func Stop(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return os.ErrProcessDone
	}
	return command.Process.Kill()
}
