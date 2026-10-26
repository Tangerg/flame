//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package filesystem

import "context"

func tryDirectory(string) (*DirectoryLease, error) { return nil, ErrLockUnsupported }

func acquireDirectory(context.Context, string) (*DirectoryLease, error) {
	return nil, ErrLockUnsupported
}
