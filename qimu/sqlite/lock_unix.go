//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package sqlite

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func acquireAuthority(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, errors.Join(fmt.Errorf("sqlite: authority already held: %w", err), file.Close())
	}
	return file, nil
}

func releaseAuthority(file *os.File) error {
	return errors.Join(syscall.Flock(int(file.Fd()), syscall.LOCK_UN), file.Close())
}
