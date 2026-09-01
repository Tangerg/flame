//go:build windows

package knowledgefile

import (
	"os"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"golang.org/x/sys/windows"
)

func syncCommittedDirectory(root *os.Root, path string) {
	directory, _, err := fileinput.OpenDirectoryAt(root, path)
	if err != nil {
		return
	}
	_ = windows.FlushFileBuffers(windows.Handle(directory.Fd()))
	_ = directory.Close()
}
