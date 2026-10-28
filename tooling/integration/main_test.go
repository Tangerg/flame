package integration

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
)

func TestMain(tests *testing.M) {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, source, _, _ := runtime.Caller(0)
	repository := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	runner, err := localruntime.RunnerExecutable(executable)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	buildRunner := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-trimpath", "-o", runner, "github.com/Tangerg/flame/qimu/runner/cmd/qimu-runner")
	buildRunner.Dir = filepath.Join(repository, "tooling")
	buildRunner.Env = append(os.Environ(), "GOWORK=off")
	buildRunner.Stdout, buildRunner.Stderr = os.Stdout, os.Stderr
	if err := buildRunner.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	companion := exec.Command("node", filepath.Join(repository, "tooling/node-companion.mjs"), "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", filepath.Dir(executable), "--offline")
	companion.Stdout, companion.Stderr = os.Stdout, os.Stderr
	if err := companion.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	packages := filepath.Join(filepath.Dir(executable), "plugins")
	command := exec.Command("node", filepath.Join(repository, "plugins/scripts/build-packages.mjs"), "--os", runtime.GOOS, "--arch", runtime.GOARCH, "--output", packages)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Setenv("FLAME_TEST_FIRST_PARTY_DIRECTORY", packages); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	status := tests.Run()
	if err := os.RemoveAll(packages); err != nil {
		fmt.Fprintln(os.Stderr, err)
		status = 1
	}
	if err := os.RemoveAll(filepath.Join(filepath.Dir(executable), "runtimes")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		status = 1
	}
	if err := os.Remove(runner); err != nil {
		fmt.Fprintln(os.Stderr, err)
		status = 1
	}
	os.Exit(status)
}
func tempRuntimeDirectory(t *testing.T) string {
	t.Helper()
	// The runner's Unix endpoint has a platform pathname limit independent
	// of the full integration test and subtest names.
	directory, err := os.MkdirTemp("", "flame-integration-")
	if err != nil {
		t.Fatal(err)
	}
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
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	return directory
}
