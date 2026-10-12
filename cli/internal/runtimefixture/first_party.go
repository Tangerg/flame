package runtimefixture

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func BuildFirstPartyPackages(ctx context.Context, directory string) error {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("locate first-party test sources")
	}
	script := filepath.Join(filepath.Dir(file), "../../..", "plugins", "build.mjs")
	command := exec.CommandContext(ctx, "node", script, "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", directory)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build first-party test packages: %w", err)
	}
	return nil
}

func RunWithFirstPartyPackages(tests *testing.M, directoryForExecutable func(string) (string, error)) int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	directory, err := directoryForExecutable(executable)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if os.Getenv("FLAME_TEST_FIRST_PARTY_DIRECTORY") == directory {
		return tests.Run()
	}
	status := 1
	if err := BuildFirstPartyPackages(context.Background(), directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if err := os.Setenv("FLAME_TEST_FIRST_PARTY_DIRECTORY", directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		status = tests.Run()
	}
	if err := os.RemoveAll(directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return status
}

// Runtime keeps admitted releases immutable; teardown alone may unseal this test-owned root.
func TempDirectory(t testing.TB) string {
	t.Helper()
	directory := t.TempDir()
	t.Cleanup(func() {
		if err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return os.Chmod(path, 0700)
			}
			return nil
		}); err != nil {
			t.Error(err)
		}
	})
	return directory
}
