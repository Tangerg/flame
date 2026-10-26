// Package advisorylock provides process-scoped filesystem leases for Infra
// components that need one cross-process linearization point.
package filesystem

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
)

// ErrUnsupported reports a platform without the required locking primitive.
var ErrLockUnsupported = errors.New("advisory lock: unsupported platform")

// Lease owns one advisory lock's release and its settled cleanup result.
type DirectoryLease struct {
	mu      sync.Mutex
	release func() (done bool, err error)
	err     error
}

func newLease(release func() (done bool, err error)) *DirectoryLease {
	return &DirectoryLease{release: release}
}

// Release retries only while the owned resource permits another attempt.
// Once cleanup settles, repeated calls return its result without more OS calls.
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

// TryDirectory acquires a non-blocking exclusive lease for the physical
// directory identity. It creates no files in the target tree. A nil lease and
// nil error mean contention; failures include owned-descriptor cleanup errors.
func TryDirectoryLease(directory string) (*DirectoryLease, error) {
	physical, err := resolveDirectory(directory)
	if err != nil {
		return nil, err
	}
	return tryDirectory(physical)
}

// AcquireDirectory waits until the physical directory identity is exclusively
// leased or ctx ends. Directory leases create no files in the target tree.
func AcquireDirectoryLease(ctx context.Context, directory string) (*DirectoryLease, error) {
	if ctx == nil {
		return nil, errors.New("advisory lock: context is required")
	}
	if err := cancellation.Err(ctx); err != nil {
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
