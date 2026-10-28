package testsupport

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"

	localruntime "github.com/Tangerg/flame/packages/local-runtime"
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
	if err := PrepareRuntimeCompanions(executable); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
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
	if err := os.RemoveAll(filepath.Join(filepath.Dir(executable), "runtimes")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.Remove(filepath.Join(filepath.Dir(executable), "qimu-runner")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return status
}

// PrepareRuntimeCompanions equips a separately built Runtime test executable
// with the same process runner and pinned interpreter as its real deployment.
func PrepareRuntimeCompanions(executable string) error {
	if err := prepareRunner(executable); err != nil {
		return err
	}
	return prepareNode(executable)
}

func RunWithNode(tests *testing.M) int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := prepareNode(executable); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	status := tests.Run()
	if err := os.RemoveAll(filepath.Join(filepath.Dir(executable), "runtimes")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return status
}

func prepareNode(executable string) error {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return fmt.Errorf("locate node companion test preparation")
	}
	script := filepath.Join(filepath.Dir(file), "../../..", "tooling", "node-companion.mjs")
	command := exec.Command("node", script, "--os", goruntime.GOOS, "--arch", goruntime.GOARCH, "--output", filepath.Dir(executable), "--offline")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

// TempDirectory joins immutable-release cleanup after the Runtime has closed.
// Runtime durability stays sealed in production; the test owns this entire root.
func TempDirectory(t testing.TB) string {
	t.Helper()
	// Runtime companions use Unix sockets whose sockaddr capacity includes
	// the full data-directory path, including the fixture's parent name.
	directory, err := os.MkdirTemp("", "flame-test-")
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

func prepareRunner(executable string) error {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return fmt.Errorf("locate runner companion sources")
	}
	runner, err := localruntime.RunnerExecutable(executable)
	if err != nil {
		return err
	}
	command := exec.Command(filepath.Join(goruntime.GOROOT(), "bin", "go"), "build", "-mod=readonly", "-o", runner, "github.com/Tangerg/flame/qimu/runner/cmd/qimu-runner")
	command.Dir = filepath.Join(filepath.Dir(file), "../..")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func RunnerExecutable(t testing.TB) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runner, err := localruntime.RunnerExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}
