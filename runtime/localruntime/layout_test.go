package localruntime

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestDefaultDataDirectoryOwnsLocalDeploymentLayout pins the layout README
// publishes: Runtime state lives under $FLAME_HOME/runtime, defaulting to
// ~/.flame/runtime. A consumer that resolved one segment itself would read a
// different database and a token the serving process never writes.
func TestDefaultDataDirectoryOwnsLocalDeploymentLayout(t *testing.T) {
	home := t.TempDir()
	directory, err := DefaultDataDirectory(home)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join(home, ".flame", "runtime")
	if directory.Path() != wantRoot {
		t.Fatalf("path = %q, want %q", directory.Path(), wantRoot)
	}
	if directory.DatabasePath() != filepath.Join(wantRoot, "flame.db") {
		t.Fatalf("database path = %q", directory.DatabasePath())
	}
	if directory.LocalTokenPath() != filepath.Join(wantRoot, "local-token") {
		t.Fatalf("token path = %q", directory.LocalTokenPath())
	}
}

// TestDataDirectoryUnderAgreesWithTheDefault keeps the two entry paths on one
// layout: an explicit FLAME_HOME and a derived one must land in the same place.
func TestDataDirectoryUnderAgreesWithTheDefault(t *testing.T) {
	home := t.TempDir()
	derived, err := DefaultDataDirectory(home)
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := DataDirectoryUnder(filepath.Join(home, ".flame"))
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Path() != derived.Path() {
		t.Fatalf("explicit product root = %q, derived = %q", explicit.Path(), derived.Path())
	}
	for _, path := range []string{"", "relative"} {
		if _, err := DataDirectoryUnder(path); !errors.Is(err, ErrInvalidDataDirectory) {
			t.Fatalf("DataDirectoryUnder(%q) error = %v", path, err)
		}
	}
}

func TestDataDirectoryRejectsUnownedPaths(t *testing.T) {
	for _, path := range []string{"", "relative"} {
		if _, err := DataDirectoryAt(path); !errors.Is(err, ErrInvalidDataDirectory) {
			t.Fatalf("DataDirectoryAt(%q) error = %v", path, err)
		}
	}
	if _, err := DefaultDataDirectory(""); !errors.Is(err, ErrInvalidDataDirectory) {
		t.Fatalf("DefaultDataDirectory error = %v", err)
	}
}
