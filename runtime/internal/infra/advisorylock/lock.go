// Package advisorylock provides process-scoped filesystem leases for Infra
// components that need one cross-process linearization point.
package advisorylock

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/cancellation"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

// ErrUnsupported reports a platform without the required locking primitive.
var ErrUnsupported = errors.New("advisory lock: unsupported platform")

// Lease owns one advisory lock's release and its settled cleanup result.
type Lease struct {
	mu      sync.Mutex
	release func() (done bool, err error)
	err     error
}

func newLease(release func() (done bool, err error)) *Lease {
	return &Lease{release: release}
}

// Release retries only while the owned resource permits another attempt.
// Once cleanup settles, repeated calls return its result without more OS calls.
func (l *Lease) Release() error {
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

// TryFile exclusively locks the first byte of an already-open regular file.
// It consumes the descriptor: the lease closes it on release, or acquisition
// closes it before returning no lease. A nil lease and nil error mean contention.
func TryFile(file *os.File) (*Lease, error) {
	if file == nil {
		return nil, errors.New("advisory lock: file is required")
	}
	return tryFile(file)
}

// TrySharedFile shared-locks the first byte of an already-open regular file.
// Multiple shared holders may coexist, while an exclusive [TryFile] holder
// excludes them. It consumes the descriptor under [TryFile]'s lifetime contract.
// A nil lease and nil error mean contention.
func TrySharedFile(file *os.File) (*Lease, error) {
	if file == nil {
		return nil, errors.New("advisory lock: file is required")
	}
	return trySharedFile(file)
}

// Closing the owned descriptor ends its usable lifetime even when Close fails.
// Always attempt both release paths, preserving their diagnostics without retrying
// a retired descriptor.
func releaseOwnedFile(file *os.File, unlock func() error) error {
	unlockErr := unlock()
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}

// TryDirectory acquires a non-blocking exclusive lease for the physical
// directory identity. It creates no files in the target tree. A nil lease and
// nil error mean contention; failures include owned-descriptor cleanup errors.
func TryDirectory(directory string) (*Lease, error) {
	physical, err := resolveDirectory(directory)
	if err != nil {
		return nil, err
	}
	return tryDirectory(physical)
}

// AcquireDirectory waits until the physical directory identity is exclusively
// leased or ctx ends. Directory leases create no files in the target tree.
func AcquireDirectory(ctx context.Context, directory string) (*Lease, error) {
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
	physical, err := pathidentity.Resolve("", directory)
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
