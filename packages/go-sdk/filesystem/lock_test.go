package filesystem_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

func TestDirectoryLeaseUsesPhysicalIdentityAndWaitsForRelease(t *testing.T) {
	directory := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	first, err := filesystem.TryDirectoryLease(directory)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := filesystem.TryDirectoryLease(alias); lease != nil || err != nil {
		t.Fatalf("alias contention = (%v, %v), want no lease or error", lease, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	waiting := make(chan *filesystem.DirectoryLease, 1)
	failures := make(chan error, 1)
	go func() {
		lease, err := filesystem.AcquireDirectoryLease(ctx, alias)
		if err != nil {
			failures <- err
			return
		}
		waiting <- lease
	}()
	select {
	case err := <-failures:
		t.Fatal(err)
	case lease := <-waiting:
		_ = lease.Release()
		t.Fatal("waiting acquisition succeeded before release")
	case <-time.After(20 * time.Millisecond):
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-failures:
		t.Fatal(err)
	case lease := <-waiting:
		if err := lease.Release(); err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(context.Cause(ctx))
	}
}

func TestDirectoryLeaseCancellationAndIdempotentRelease(t *testing.T) {
	directory := t.TempDir()
	first, err := filesystem.TryDirectoryLease(directory)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := filesystem.AcquireDirectoryLease(canceled, directory); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquisition = %v, want context.Canceled", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := filesystem.AcquireDirectoryLease(canceled, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("uncontended canceled acquisition = %v, want context.Canceled", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
}
