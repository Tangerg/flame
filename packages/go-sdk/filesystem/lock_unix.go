//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package filesystem

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/Tangerg/flame/qimu/cancellation"
	"golang.org/x/sys/unix"
)

const directoryLockRetryInterval = time.Millisecond

func lockFile(file *os.File) (bool, error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func acquireDirectory(ctx context.Context, directory string) (*DirectoryLease, error) {
	file, _, err := OpenDirectory(directory)
	if err != nil {
		return nil, err
	}
	retry := time.NewTicker(directoryLockRetryInterval)
	defer retry.Stop()
	for {
		if err := cancellation.Err(ctx); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		acquired, err := lockFile(file)
		if err != nil {
			return nil, errors.Join(err, file.Close())
		}
		if acquired {
			return newFileLease(file), nil
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(cancellation.Err(ctx), file.Close())
		case <-retry.C:
		}
	}
}

func tryDirectory(directory string) (*DirectoryLease, error) {
	file, _, err := OpenDirectory(directory)
	if err != nil {
		return nil, err
	}
	if acquired, err := lockFile(file); err != nil || !acquired {
		return nil, errors.Join(err, file.Close())
	}
	return newFileLease(file), nil
}

func newFileLease(file *os.File) *DirectoryLease {
	return newLease(func() (bool, error) {
		return true, releaseOwnedFile(file, func() error {
			return unix.Flock(int(file.Fd()), unix.LOCK_UN)
		})
	})
}

// Closing the owned descriptor ends its usable lifetime even when Close fails.
// Always attempt both release paths, preserving their diagnostics without retrying
// a retired descriptor.
func releaseOwnedFile(file *os.File, unlock func() error) error {
	unlockErr := unlock()
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}
