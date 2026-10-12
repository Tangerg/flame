package runtimebinding

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(tests *testing.M) {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	directory := filepath.Join(filepath.Dir(executable), "plugins")
	if os.Getenv("FLAME_TEST_FIRST_PARTY_DIRECTORY") == directory {
		os.Exit(tests.Run())
	}
	command := exec.Command("node", "../../../../plugins/build.mjs", "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", directory)
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
		status = 1
	}
	os.Exit(status)
}

func tempDirectory(t testing.TB) string {
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
