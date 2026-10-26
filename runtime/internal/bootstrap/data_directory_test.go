package bootstrap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDataDirectoryLeaseProcess(t *testing.T) {
	path := os.Getenv("FLAME_TEST_DIRECTORY_LEASE")
	if path == "" {
		return
	}
	owner, err := openDataDirectory(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	fmt.Fprintln(os.Stdout, "owned")
	if _, err := bufio.NewReader(os.Stdin).ReadByte(); err != nil {
		t.Fatal(err)
	}
	os.Exit(78)
}

func TestDataDirectoryLeaseExcludesOtherProcessesUntilOwnerExit(t *testing.T) {
	path := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDataDirectoryLeaseProcess$")
	command.Env = append(os.Environ(), "FLAME_TEST_DIRECTORY_LEASE="+path)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait() })
	if ready, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || ready != "owned\n" {
		t.Fatalf("directory owner readiness = %q, %v", ready, err)
	}
	for _, target := range []string{path, alias} {
		if unexpected, err := openDataDirectory(t.Context(), target); !errors.Is(err, errDataDirectoryInUse) {
			if unexpected != nil {
				_ = unexpected.Close()
			}
			t.Fatalf("another process acquired %q: %v", target, err)
		}
	}
	if _, err := stdin.Write([]byte{'x'}); err != nil {
		t.Fatal(err)
	}
	var exited *exec.ExitError
	if err := command.Wait(); !errors.As(err, &exited) || exited.ExitCode() != 78 {
		t.Fatalf("owner exit = %v", err)
	}
	successor, err := openDataDirectory(t.Context(), alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := successor.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDataDirectoryCanonicalizesAndProtectsDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatalf("create alias: %v", err)
	}
	setup, err := openDataDirectory(t.Context(), alias)
	if err != nil {
		t.Fatalf("acquire setup: %v", err)
	}
	defer func() { _ = setup.Close() }()
	physical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve directory: %v", err)
	}
	if setup.directory != physical {
		t.Fatalf("canonical directory = %q, want %q", setup.directory, physical)
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
	first, err := openDataDirectory(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	for _, target := range []string{directory, alias} {
		if _, err := openDataDirectory(t.Context(), target); !errors.Is(err, errDataDirectoryInUse) {
			t.Fatalf("second writer for %s: %v", target, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := openDataDirectory(t.Context(), alias)
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
	if _, err := openDataDirectory(ctx, directory); !errors.Is(err, context.Canceled) {
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
	directory, err := openDataDirectory(t.Context(), path)
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
	if err := directory.validate(); err == nil {
		t.Fatal("replacement root retained write authority")
	}
}
