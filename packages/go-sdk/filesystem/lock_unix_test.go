//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
)

func TestOwnedDirectoryReleasePreservesUnlockFailureAndClosesFile(t *testing.T) {
	file, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("unlock failed")
	if err := releaseOwnedFile(file, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("release = %v, want unlock failure", err)
	}
	if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("file after failed unlock = %v, want closed", err)
	}
}

func TestOwnedDirectoryReleasePreservesCloseFailure(t *testing.T) {
	file, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := releaseOwnedFile(file, func() error { return nil }); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("release = %v, want close failure", err)
	}
}

func TestDirectoryAcquisitionPreservesCanceledLifetime(t *testing.T) {
	failure := errors.New("setup withdrawn")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(failure)
	lease, err := AcquireDirectoryLease(ctx, filepath.Join(t.TempDir(), "unavailable"))
	if lease != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
		t.Fatalf("acquisition = (%v, %v), want cancellation and cause", lease, err)
	}
}

func TestDirectoryWaitPreservesCanceledLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		directory := t.TempDir()
		owner, err := TryDirectoryLease(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := owner.Release(); err != nil {
				t.Error(err)
			}
		}()
		failure := errors.New("setup withdrawn")
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		done := make(chan error, 1)
		go func() {
			lease, err := AcquireDirectoryLease(ctx, directory)
			if lease != nil {
				err = errors.Join(errors.New("canceled waiter acquired the directory"), lease.Release())
			}
			done <- err
		}()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("contended acquisition settled before cancellation: %v", err)
		default:
		}
		cancel(failure)
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
			t.Fatalf("wait = %v, want cancellation and cause", err)
		}
	})
}
