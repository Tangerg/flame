//go:build windows

package filesystem

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"golang.org/x/sys/windows"
)

const directoryLockWaitMilliseconds uint32 = 10

func acquireDirectory(ctx context.Context, directory string) (*Lease, error) {
	return directoryLease(ctx, directory, false)
}

func tryDirectory(directory string) (*Lease, error) {
	return directoryLease(nil, directory, true)
}

func directoryLease(ctx context.Context, directory string, nonblocking bool) (*Lease, error) {
	type result struct {
		lease *Lease
		err   error
	}
	resultChannel := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		handle, err := acquireDirectoryHandle(ctx, directory, nonblocking)
		if err != nil || handle == 0 {
			resultChannel <- result{err: err}
			return
		}
		type releaseResult struct {
			done bool
			err  error
		}
		requests := make(chan chan releaseResult)
		lease := newLease(func() (bool, error) {
			response := make(chan releaseResult, 1)
			requests <- response
			result := <-response
			return result.done, result.err
		})
		resultChannel <- result{lease: lease}
		for response := range requests {
			err := windows.ReleaseMutex(handle)
			if err != nil {
				response <- releaseResult{err: err}
				continue
			}
			response <- releaseResult{done: true, err: windows.CloseHandle(handle)}
			return
		}
	}()
	acquired := <-resultChannel
	return acquired.lease, acquired.err
}

func acquireDirectoryHandle(ctx context.Context, directory string, nonblocking bool) (windows.Handle, error) {
	directoryHandle, _, err := OpenDirectory(directory)
	if err != nil {
		return 0, err
	}
	var identity windows.ByHandleFileInformation
	identityErr := windows.GetFileInformationByHandle(windows.Handle(directoryHandle.Fd()), &identity)
	closeErr := directoryHandle.Close()
	if err := errors.Join(identityErr, closeErr); err != nil {
		return 0, err
	}
	name := fmt.Sprintf(
		`Global\Flame-Advisory-Directory-%08x-%08x%08x`,
		identity.VolumeSerialNumber, identity.FileIndexHigh, identity.FileIndexLow,
	)
	nameUTF16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateMutex(nil, false, nameUTF16)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			err = errors.Join(err, windows.CloseHandle(handle))
		}
		return 0, err
	}
	for {
		if !nonblocking {
			if err := cancellation.Err(ctx); err != nil {
				return 0, errors.Join(err, windows.CloseHandle(handle))
			}
		}
		result, err := windows.WaitForSingleObject(handle, directoryLockWaitMilliseconds)
		if err != nil {
			return 0, errors.Join(err, windows.CloseHandle(handle))
		}
		switch result {
		case windows.WAIT_OBJECT_0, windows.WAIT_ABANDONED:
			return handle, nil
		case uint32(windows.WAIT_TIMEOUT):
			if nonblocking {
				return 0, windows.CloseHandle(handle)
			}
			select {
			case <-ctx.Done():
				return 0, errors.Join(cancellation.Err(ctx), windows.CloseHandle(handle))
			default:
			}
		default:
			return 0, errors.Join(fmt.Errorf("advisory lock: unexpected wait result %d", result), windows.CloseHandle(handle))
		}
	}
}
