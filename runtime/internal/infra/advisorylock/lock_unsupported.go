//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package advisorylock

import (
	"context"
	"errors"
	"os"
)

func tryFile(file *os.File) (*Lease, error) {
	return nil, errors.Join(ErrUnsupported, file.Close())
}

func trySharedFile(file *os.File) (*Lease, error) {
	return nil, errors.Join(ErrUnsupported, file.Close())
}

func tryDirectory(string) (*Lease, error) { return nil, ErrUnsupported }

func acquireDirectory(context.Context, string) (*Lease, error) { return nil, ErrUnsupported }
