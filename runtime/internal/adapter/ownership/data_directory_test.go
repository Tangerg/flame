package ownership

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirectoryCanonicalizesAndProtectsDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatalf("create alias: %v", err)
	}
	setup, err := OpenDataDirectory(t.Context(), alias)
	if err != nil {
		t.Fatalf("acquire setup: %v", err)
	}
	defer func() { _ = setup.Close() }()
	physical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve directory: %v", err)
	}
	if setup.Directory != physical {
		t.Fatalf("canonical directory = %q, want %q", setup.Directory, physical)
	}
	info, err := os.Stat(physical)
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", info.Mode().Perm())
	}
}

func TestDataDirectoryRefusesAnotherWriterUntilClose(t *testing.T) {
	directory := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatal(err)
	}
	first, err := OpenDataDirectory(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	for _, target := range []string{directory, alias} {
		if _, err := OpenDataDirectory(t.Context(), target); !errors.Is(err, ErrDataDirectoryInUse) {
			t.Fatalf("second writer for %s: %v", target, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenDataDirectory(t.Context(), alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDataDirectoryCanceledOpenDoesNotChangePermissions(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := OpenDataDirectory(ctx, directory); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatal("canceled writer changed directory permissions")
	}
}

func TestDataDirectoryRejectsAReplacedRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	directory, err := OpenDataDirectory(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = directory.Close() })
	if err := os.Rename(path, path+"-retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := directory.Validate(); err == nil {
		t.Fatal("replacement root retained write authority")
	}
}
