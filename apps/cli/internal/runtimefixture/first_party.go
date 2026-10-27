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

func BuildRuntimeCompanions(ctx context.Context, executable, directory, runner string) error {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("locate first-party test sources")
	}
	repository := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-o", runner, "github.com/Tangerg/flame/qimu/runner/cmd/qimu-runner")
	build.Dir = filepath.Join(repository, "apps", "cli")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build actual runner companion: %w", err)
	}
	node := exec.CommandContext(ctx, "node", filepath.Join(repository, "tooling/node-companion.mjs"), "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", filepath.Dir(executable), "--offline")
	node.Stdout, node.Stderr = os.Stdout, os.Stderr
	if err := node.Run(); err != nil {
		return fmt.Errorf("prepare pinned Node companion: %w", err)
	}
	script := filepath.Join(repository, "plugins", "scripts", "build-packages.mjs")
	command := exec.CommandContext(ctx, "node", script, "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", directory)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build first-party test packages: %w", err)
	}
	return nil
}

func RunWithRuntimeCompanions(tests *testing.M, directoryForExecutable, runnerForExecutable func(string) (string, error)) int {
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
	runner, err := runnerForExecutable(executable)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	status := 1
	if err := BuildRuntimeCompanions(context.Background(), executable, directory, runner); err != nil {
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
	if err := os.RemoveAll(filepath.Join(filepath.Dir(executable), "runtimes")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	err = os.Remove(runner)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return status
}

// Runtime keeps admitted releases immutable; teardown alone may unseal this test-owned root.
func TempDirectory(t testing.TB) string {
	t.Helper()
	// The production Unix control endpoint has an OS pathname bound. This
	// fixture owns a short root instead of deriving it from a long test name.
	directory, err := os.MkdirTemp("", "flame-cli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer func() {
			if err := os.RemoveAll(directory); err != nil {
				t.Error(err)
			}
		}()
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
