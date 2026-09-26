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

func TestAdmittedReplacementKeepsItsDirectoryAfterStoreClose(t *testing.T) {
	root := t.TempDir()
	store := openStore(t, root)
	// Pause an admitted write before its file I/O, then retire the Store. Its
	// independently pinned directory remains owned by that one operation.
	directory, base, err := store.replacementDirectory("inputs/prepared.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = directory.Close() }()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	body := []byte("complete input after owner close")
	if err := replaceStateFile(directory, base, body); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "inputs", "prepared.json"))
	if err != nil || !slices.Equal(got, body) {
		t.Fatalf("admitted replacement lost its pinned directory: %q, %v", got, err)
	}
	if err := store.Replace("late.json", body); err == nil {
		t.Fatal("closed Store admitted a new replacement")
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
