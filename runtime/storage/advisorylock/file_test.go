package advisorylock_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/storage/advisorylock"
)

type fileAcquisition struct {
	name string
	try  func(*os.File) (*advisorylock.Lease, error)
}

func fileAcquisitions() []fileAcquisition {
	return []fileAcquisition{
		{"exclusive", advisorylock.TryFile},
		{"shared", advisorylock.TrySharedFile},
	}
}

func openLeaseFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error(err)
		}
	})
	return file
}

func TestFileLeaseOwnsDescriptorAndSettledResult(t *testing.T) {
	for _, acquisition := range fileAcquisitions() {
		t.Run(acquisition.name, func(t *testing.T) {
			file := openLeaseFile(t, filepath.Join(t.TempDir(), "lease"))
			lease, err := acquisition.try(file)
			if err != nil || lease == nil {
				t.Fatalf("acquisition = (%v, %v), want a lease", lease, err)
			}
			if err := lease.Release(); err != nil {
				t.Fatal(err)
			}
			if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Errorf("released descriptor = %v, want closed", err)
			}
			if err := lease.Release(); err != nil {
				t.Fatalf("repeated release = %v, want settled result", err)
			}
		})
	}
}

func TestFileAcquisitionConsumesContendedDescriptor(t *testing.T) {
	for _, acquisition := range fileAcquisitions() {
		t.Run(acquisition.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lease")
			owner, err := advisorylock.TryFile(openLeaseFile(t, path))
			if err != nil || owner == nil {
				t.Fatalf("owner = (%v, %v), want an exclusive lease", owner, err)
			}
			t.Cleanup(func() {
				if err := owner.Release(); err != nil {
					t.Error(err)
				}
			})
			file := openLeaseFile(t, path)
			if lease, err := acquisition.try(file); lease != nil || err != nil {
				t.Fatalf("contention = (%v, %v), want no lease or error", lease, err)
			}
			if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Errorf("contended descriptor = %v, want closed", err)
			}
		})
	}
}

func TestFileAcquisitionRetainsConsumedDescriptorFailure(t *testing.T) {
	for _, acquisition := range fileAcquisitions() {
		t.Run(acquisition.name, func(t *testing.T) {
			file := openLeaseFile(t, filepath.Join(t.TempDir(), "lease"))
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if lease, err := acquisition.try(file); lease != nil || !errors.Is(err, os.ErrClosed) {
				t.Fatalf("acquisition = (%v, %v), want retained descriptor failure", lease, err)
			}
		})
	}
}
