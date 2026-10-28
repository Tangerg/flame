package statefile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func openStore(t *testing.T, root string) *Store {
	t.Helper()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

func TestStoreReplacesReadsListsAndRemovesState(t *testing.T) {
	store := openStore(t, t.TempDir())
	if err := store.Replace(filepath.Join("sessions", "one.json"), []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(filepath.Join("sessions", "one.json"), []byte("second")); err != nil {
		t.Fatal(err)
	}
	body, err := store.Read(filepath.Join("sessions", "one.json"), 6)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "second" {
		t.Fatalf("body = %q", body)
	}
	names, err := store.ListFiles("sessions", ".json")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names, []string{"one.json"}) {
		t.Fatalf("names = %v", names)
	}
	if err := store.Remove(filepath.Join("sessions", "one.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(filepath.Join("sessions", "one.json"), 6); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read removed state: %v", err)
	}
}

func TestStoreUsesPhysicalParentAfterSymlinkTraversal(t *testing.T) {
	directory := t.TempDir()
	physical := filepath.Join(directory, "physical")
	if err := os.MkdirAll(filepath.Join(physical, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(directory, "alias")
	if err := os.Symlink(filepath.Join(physical, "child"), alias); err != nil {
		t.Fatal(err)
	}
	store := openStore(t, alias+string(filepath.Separator)+"..")
	if err := store.Replace("state.json", []byte("original owner")); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(filepath.Join(physical, "state.json"))
	if err != nil || string(value) != "original owner" {
		t.Fatalf("physical state = %q, %v", value, err)
	}
	if other, err := Open(physical); err == nil {
		_ = other.Close()
		t.Fatal("the same physical directory acquired a competing lease")
	}
}

func TestStorePreservesWhitespaceInDirectoryIdentity(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state ")
	store := openStore(t, directory)
	if err := store.Replace("state.json", []byte("exact path")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "state.json")); err != nil {
		t.Fatal(err)
	}
}

func TestStoreListFilesFiltersAcrossReadBatches(t *testing.T) {
	root := t.TempDir()
	store := openStore(t, root)
	if err := store.Replace(filepath.Join("sessions", "one.json"), nil); err != nil {
		t.Fatal(err)
	}
	for index := range listReadBatchSize + 1 {
		name := filepath.Join(root, "sessions", fmt.Sprintf("ignored-%03d.tmp", index))
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Replace(filepath.Join("sessions", "two.json"), nil); err != nil {
		t.Fatal(err)
	}
	names, err := store.ListFiles("sessions", ".json")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"one.json", "two.json"}) {
		t.Fatalf("names = %v", names)
	}
}

func TestStoreRejectsUnsafeNamesAndNonRegularState(t *testing.T) {
	root := t.TempDir()
	store := openStore(t, root)
	for _, name := range []string{"", ".", "../outside", "/outside", `sessions\outside`} {
		if err := store.Replace(name, []byte("state")); err == nil {
			t.Fatalf("Replace(%q) unexpectedly succeeded", name)
		}
	}
	if err := os.Symlink(filepath.Join(root, "target"), filepath.Join(root, "linked.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read("linked.json", 16); err == nil {
		t.Fatal("Read accepted symbolic-link state")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "sessions")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "escaped.json"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListFiles("sessions", ".json"); err == nil {
		t.Fatal("ListFiles followed a symbolic-link state directory")
	}
}

func TestStoreRejectsSymlinksAndRequiresReopenAfterRootReplacement(t *testing.T) {
	parent := t.TempDir()
	configured := filepath.Join(parent, "state")
	store := openStore(t, configured)
	if err := store.Replace(filepath.Join("sessions", "one.json"), []byte("before")); err != nil {
		t.Fatal(err)
	}

	pinned := filepath.Join(parent, "pinned")
	if err := os.Rename(configured, pinned); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, configured); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(filepath.Join("sessions", "two.json"), []byte("inside")); err == nil {
		t.Fatal("Replace followed a symbolic-link state root")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("replacement target received state: %v", entries)
	}
	if err := os.Remove(configured); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(configured, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(filepath.Join("sessions", "two.json"), []byte("inside")); err == nil {
		t.Fatal("live Store switched ownership to a replacement root")
	}
	if entries, err := os.ReadDir(configured); err != nil || len(entries) != 0 {
		t.Fatalf("replacement root received old owner state: %v, %v", entries, err)
	}
	reopened := openStore(t, configured)
	if err := reopened.Replace(filepath.Join("sessions", "two.json"), []byte("inside")); err != nil {
		t.Fatal(err)
	}
	body, err := reopened.Read(filepath.Join("sessions", "two.json"), 16)
	if err != nil || string(body) != "inside" {
		t.Fatalf("Read() = %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(configured, "sessions", "two.json")); err != nil {
		t.Fatalf("new owner did not write the replacement root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pinned, "sessions", "one.json")); err != nil {
		t.Fatalf("original pinned state was damaged: %v", err)
	}
}

func TestStoreCloseIsIdempotentAndRejectsFurtherOperations(t *testing.T) {
	store := openStore(t, t.TempDir())
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if _, err := store.Read("state.json", 16); err == nil {
		t.Fatal("Read succeeded after Close")
	}
	if _, err := store.ListFiles("sessions", ".json"); err == nil {
		t.Fatal("ListFiles succeeded after Close")
	}
	if err := store.Replace("state.json", nil); err == nil {
		t.Fatal("Replace succeeded after Close")
	}
	if err := store.Remove("state.json"); err == nil {
		t.Fatal("Remove succeeded after Close")
	}
}

func TestOpenRequiresAnAbsoluteRoot(t *testing.T) {
	for _, root := range []string{"", "   ", "relative"} {
		if _, err := Open(root); err == nil {
			t.Fatalf("Open(%q) unexpectedly succeeded", root)
		}
	}
}

func TestStoreExclusivelyOwnsPhysicalDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{root, alias} {
		if second, err := Open(target); err == nil {
			_ = second.Close()
			t.Fatal("second writer admitted")
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentReplacementsPublishWholeFiles(t *testing.T) {
	root := t.TempDir()
	store := openStore(t, root)
	bodies := [][]byte{bytes.Repeat([]byte("a"), 64<<10), bytes.Repeat([]byte("b"), 64<<10)}
	if err := store.Replace("shared.json", bodies[0]); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, len(bodies))
	for _, body := range bodies {
		go func() {
			for range 8 {
				if err := store.Replace("shared.json", body); err != nil {
					finished <- err
					return
				}
			}
			finished <- nil
		}()
	}
	for range 32 {
		got, err := os.ReadFile(filepath.Join(root, "shared.json"))
		if err != nil || (!bytes.Equal(got, bodies[0]) && !bytes.Equal(got, bodies[1])) {
			t.Fatalf("concurrent replacement exposed partial bytes: length %d, %v", len(got), err)
		}
	}
	for range bodies {
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
	}
}
