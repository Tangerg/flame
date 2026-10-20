package testsupport

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
)

// RunWithFirstPartyPackages gives the test executable the same companion
// distribution as a shipping Runtime, without consulting a user's deployment.
func RunWithFirstPartyPackages(tests *testing.M) int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	directory := filepath.Join(filepath.Dir(executable), "plugins")
	if os.Getenv("FLAME_TEST_FIRST_PARTY_DIRECTORY") == directory {
		return tests.Run()
	}
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "locate first-party test sources")
		return 1
	}
	script := filepath.Join(filepath.Dir(file), "../../..", "plugins", "scripts", "build-packages.mjs")
	command := exec.Command("node", script, "--os", goruntime.GOOS, "--arch", goruntime.GOARCH, "--output", directory)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	status := 1
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		if err := os.Setenv("FLAME_TEST_FIRST_PARTY_DIRECTORY", directory); err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			status = tests.Run()
		}
	}
	if err := os.RemoveAll(directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return status
}

// TempDirectory joins immutable-release cleanup after the Runtime has closed.
// Runtime durability stays sealed in production; the test owns this entire root.
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
