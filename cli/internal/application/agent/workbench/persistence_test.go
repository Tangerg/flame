package workbench

import (
	"errors"
	"os"
	"testing"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"
)

type closeTrackingPersistence struct {
	closed int
}

func (*closeTrackingPersistence) Read(string, int64) ([]byte, error) { return nil, os.ErrNotExist }
func (*closeTrackingPersistence) List(string) ([]string, error)      { return nil, os.ErrNotExist }
func (*closeTrackingPersistence) Replace(string, []byte) error       { return nil }
func (*closeTrackingPersistence) Remove(string) error                { return nil }
func (p *closeTrackingPersistence) Close() error {
	p.closed++
	return nil
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
	store, err := Open(persistence, config)
	if err != nil {
		return nil, errors.Join(err, persistence.Close())
	}
	return store, nil
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
