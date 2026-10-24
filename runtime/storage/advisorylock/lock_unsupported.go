//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package advisorylock

import "context"

func tryDirectory(string) (*Lease, error) { return nil, ErrUnsupported }

func acquireDirectory(context.Context, string) (*Lease, error) { return nil, ErrUnsupported }
