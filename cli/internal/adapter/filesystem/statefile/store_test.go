package statefile

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestStoreReplacesReadsListsAndRemovesState(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
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
	names, err := store.List("sessions")
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

func TestStoreRejectsUnsafeNamesAndNonRegularState(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
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
	if _, err := store.List("sessions"); err == nil {
		t.Fatal("List followed a symbolic-link state directory")
	}
}

func TestOpenRequiresAnAbsoluteRoot(t *testing.T) {
	for _, root := range []string{"", "   ", "relative"} {
		if _, err := Open(root); err == nil {
			t.Fatalf("Open(%q) unexpectedly succeeded", root)
		}
	}
}
