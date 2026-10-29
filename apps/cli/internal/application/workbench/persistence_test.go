package workbench

import (
	"errors"
	"os"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

type closeTrackingPersistence struct {
	closed     int
	listErr    error
	confirmErr error
}

func (p *closeTrackingPersistence) ConfirmDurability() error { return p.confirmErr }

func TestOpenClosesPersistenceWhenDurabilityCannotBeConfirmed(t *testing.T) {
	cause := errors.New("directory sync unavailable")
	persistence := &closeTrackingPersistence{confirmErr: cause}
	store, err := Open(persistence, Config{})
	if store != nil || !errors.Is(err, cause) || persistence.closed != 1 {
		t.Fatalf("unconfirmed store opened or leaked: store=%v error=%v closes=%d", store, err, persistence.closed)
	}
}

func (*closeTrackingPersistence) Read(string, int64) ([]byte, error) { return nil, os.ErrNotExist }
func (p *closeTrackingPersistence) ListFiles(string, string) ([]string, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	return nil, os.ErrNotExist
}
func (*closeTrackingPersistence) Replace(string, []byte) filesystem.CommitResult {
	return filesystem.CommitResult{State: filesystem.CommitDurable}
}
func (*closeTrackingPersistence) Remove(string) filesystem.CommitResult {
	return filesystem.CommitResult{State: filesystem.CommitDurable}
}
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

func OpenDirectory(t testing.TB, directory string, config Config) (*Store, error) {
	t.Helper()
	persistence, err := statefile.Open(directory)
	if err != nil {
		return nil, err
	}
	store, err := Open(persistence, config)
	if err == nil {
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	return store, err
}

type removeFailurePersistence struct {
	Persistence
	name    string
	enabled bool
}

func (r *removeFailurePersistence) Remove(name string) filesystem.CommitResult {
	if r.enabled && name == r.name {
		return filesystem.CommitResult{Err: errors.New("injected remove failure")}
	}
	return r.Persistence.Remove(name)
}

func (r *removeFailurePersistence) Close() error { return closePersistence(r.Persistence) }
