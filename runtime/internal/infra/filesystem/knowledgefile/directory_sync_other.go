//go:build !windows

package knowledgefile

import (
	"errors"
	"os"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

func syncCommittedDirectory(root *os.Root, path string) {
	directory, _, err := fileinput.OpenDirectoryAt(root, path)
	if err != nil {
		return
	}
	_ = errors.Join(directory.Sync(), directory.Close())
}
