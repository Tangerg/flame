//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package advisorylock

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/Tangerg/flame/runtime/internal/cancellation"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"golang.org/x/sys/unix"
)

const directoryLockRetryInterval = time.Millisecond

func tryFile(file *os.File) (*Lease, error) {
	return tryFileMode(file, unix.LOCK_EX)
}

func trySharedFile(file *os.File) (*Lease, error) {
	return tryFileMode(file, unix.LOCK_SH)
}

func tryFileMode(file *os.File, mode int) (*Lease, error) {
	if acquired, err := lockFile(file, mode); err != nil || !acquired {
		return nil, err
	}
	return newLease(func() (bool, error) {
		err := unix.Flock(int(file.Fd()), unix.LOCK_UN)
		return err == nil, err
	}), nil
}

func lockFile(file *os.File, mode int) (bool, error) {
	if err := unix.Flock(int(file.Fd()), mode|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func acquireDirectory(ctx context.Context, directory string) (*Lease, error) {
	file, _, err := fileinput.OpenDirectory(directory)
	if err != nil {
		return nil, err
	}
	retry := time.NewTicker(directoryLockRetryInterval)
	defer retry.Stop()
	for {
		if err := cancellation.Err(ctx); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		acquired, err := lockFile(file, unix.LOCK_EX)
		if err != nil {
			return nil, errors.Join(err, file.Close())
		}
		if acquired {
			return newDirectoryLease(file), nil
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(cancellation.Err(ctx), file.Close())
		case <-retry.C:
		}
	}
}

func tryDirectory(directory string) (*Lease, error) {
	file, _, err := fileinput.OpenDirectory(directory)
	if err != nil {
		return nil, err
	}
	if acquired, err := lockFile(file, unix.LOCK_EX); err != nil || !acquired {
		return nil, errors.Join(err, file.Close())
	}
	return newDirectoryLease(file), nil
}

func newDirectoryLease(file *os.File) *Lease {
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
