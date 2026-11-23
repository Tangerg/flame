package filesystem

import (
	"context"
	"errors"
	"os"
	"sync"
)

var ErrLockUnsupported = errors.New("advisory lock: unsupported platform")

type DirectoryLease struct {
	mu      sync.Mutex
	release func() (done bool, err error)
	err     error
}

func newLease(release func() (done bool, err error)) *DirectoryLease {
	return &DirectoryLease{release: release}
}

func (l *DirectoryLease) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.release == nil {
		return l.err
	}
	done, err := l.release()
	if done {
		l.release = nil
		l.err = err
	}
	return err
}

func TryDirectoryLease(directory string) (*DirectoryLease, error) {
	physical, err := resolveDirectory(directory)
	if err != nil {
		return nil, err
	}
	return tryDirectory(physical)
}

func AcquireDirectoryLease(ctx context.Context, directory string) (*DirectoryLease, error) {
	if ctx == nil {
		return nil, errors.New("advisory lock: context is required")
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, err
	}
	physical, err := resolveDirectory(directory)
	if err != nil {
		return nil, err
	}
	return acquireDirectory(ctx, physical)
}

func resolveDirectory(directory string) (string, error) {
	physical, err := ResolvePath("", directory)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(physical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("advisory lock: identity is not a directory")
	}
	return physical, nil
}
