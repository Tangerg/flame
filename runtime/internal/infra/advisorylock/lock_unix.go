//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package advisorylock

import (
	"context"
	"errors"
	"os"
	"time"

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
	if err := unix.Flock(int(file.Fd()), mode|unix.LOCK_NB); err != nil {
		if isContention(err) {
			return nil, ErrContended
		}
		return nil, err
	}
	return newLease(func() error { return unix.Flock(int(file.Fd()), unix.LOCK_UN) }), nil
}

func acquireDirectory(ctx context.Context, directory string) (*Lease, error) {
	file, _, err := fileinput.OpenDirectory(directory)
	if err != nil {
		return nil, err
	}
	retry := time.NewTicker(directoryLockRetryInterval)
	defer retry.Stop()
	for {
		if cause := context.Cause(ctx); cause != nil {
			_ = file.Close()
			return nil, cause
		}
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return newLease(func() error {
				return releaseOwnedFile(file, func() error {
					return unix.Flock(int(file.Fd()), unix.LOCK_UN)
				})
			}), nil
		}
		if !isContention(err) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, context.Cause(ctx)
		case <-retry.C:
		}
	}
}

func tryDirectory(directory string) (*Lease, error) {
	file, _, err := fileinput.OpenDirectory(directory)
	if err != nil {
		return nil, err
	}
	lease, err := tryFile(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return newLease(func() error {
		return releaseOwnedFile(file, lease.Release)
	}), nil
}

// releaseOwnedFile gives a directory lease two independent release paths:
// explicit unlock and closing its Runtime-owned descriptor. Either one releases
// a Unix flock, so a failed unlock must never prevent the close fallback.
func releaseOwnedFile(file *os.File, unlock func() error) error {
	unlockErr := unlock()
	closeErr := file.Close()
	if unlockErr == nil || closeErr == nil {
		return nil
	}
	return errors.Join(unlockErr, closeErr)
}

func isContention(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
