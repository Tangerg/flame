//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package workspace

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"golang.org/x/sys/unix"
)

const taskWriterDirectoryDescriptor = 3

func retainTaskWriterDirectory(command *exec.Cmd, directory string) (*os.File, error) {
	file, _, err := filesystem.OpenDirectory(directory)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_SH|unix.LOCK_NB); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	// The worker keeps the open description across Runtime exit. New Runtime
	// startup waits on an exclusive lease, while writers of disjoint trees
	// can hold shared leases together.
	command.ExtraFiles = []*os.File{file}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return file, nil
}

func inheritedTaskWriterDirectory() *os.File {
	unix.CloseOnExec(taskWriterDirectoryDescriptor)
	return os.NewFile(taskWriterDirectoryDescriptor, taskWriteDirectory)
}
