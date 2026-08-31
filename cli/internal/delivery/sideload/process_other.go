//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package sideload

import "os/exec"

func configureProcess(command *exec.Cmd) {
	command.WaitDelay = processWaitDelay
}
