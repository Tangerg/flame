package workbench

import (
	"errors"
	"os"
	"testing"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"
)

type closeTrackingPersistence struct {
	closed  int
	listErr error
}

func (*closeTrackingPersistence) Read(string, int64) ([]byte, error) { return nil, os.ErrNotExist }
func (p *closeTrackingPersistence) ListFiles(string, string) ([]string, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	return nil, os.ErrNotExist
}
func (*closeTrackingPersistence) Replace(string, []byte) error { return nil }
func (*closeTrackingPersistence) Remove(string) error          { return nil }
func (p *closeTrackingPersistence) Close() error {
	p.closed++
	return nil
}

// TestOpenClosesPersistenceWhenLoadingFails pins the ownership half that used
// to live in every caller: the handle is the Store's from the call, so a failed
// load releases it rather than leaving the caller to remember.
func TestOpenClosesPersistenceWhenLoadingFails(t *testing.T) {
	persistence := &closeTrackingPersistence{listErr: errors.New("state directory is unreadable")}
	if _, err := Open(persistence, Config{}); err == nil {
		t.Fatal("Open succeeded with an unreadable state directory")
	}
	if persistence.closed != 1 {
		t.Fatalf("persistence close count = %d, want 1", persistence.closed)
	}
}

func TestStoreClosesOwnedPersistenceOnce(t *testing.T) {
	persistence := new(closeTrackingPersistence)
	store, err := Open(persistence, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if persistence.closed != 1 {
		t.Fatalf("persistence close count = %d, want 1", persistence.closed)
	}
}

func OpenDirectory(directory string, config Config) (*Store, error) {
	persistence, err := statefile.Open(directory)
	if err != nil {
		return nil, err
	}
	return Open(persistence, config)
}

type removeFailurePersistence struct {
	Persistence
	name    string
	enabled bool
}

func (r *removeFailurePersistence) Remove(name string) error {
	if r.enabled && name == r.name {
		return errors.New("injected remove failure")
	}
	return r.Persistence.Remove(name)
}
