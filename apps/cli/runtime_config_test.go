package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeConfigDirectoriesUseOnlyExplicitSource(t *testing.T) {
	explicitDirectory := t.TempDir()
	t.Setenv(runtimeConfigDirectoryEnvironment, explicitDirectory)

	directories := runtimeConfigDirectories()
	want := []string{explicitDirectory}
	if len(directories) != len(want) || directories[0] != want[0] {
		t.Fatalf("directories = %v, want %v", directories, want)
	}
}

func TestEmbeddedRuntimeValidatesItsConfigOnlyWhenSelected(t *testing.T) {
	t.Setenv(runtimeConfigDirectoryEnvironment, "relative/config")
	owner, err := newRuntimeOwnerAt(t.TempDir())
	if err != nil {
		t.Fatalf("client construction interpreted an unselected Runtime config: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	if _, err := owner.Connection(t.Context(), ""); err == nil {
		t.Fatal("embedded Runtime accepted relative configuration directory")
	}
}

func TestRuntimeConfigDirectoriesIgnoreWorkingDirectoryConfig(t *testing.T) {
	t.Setenv(runtimeConfigDirectoryEnvironment, "")
	root := t.TempDir()
	configDirectory := filepath.Join(root, "runtime", "config")
	cliDirectory := filepath.Join(root, "cli")
	for _, directory := range []string{configDirectory, cliDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(root, "go.work"):                "go 1.27.0\n",
		filepath.Join(root, "runtime", "go.mod"):      "module example/runtime\n",
		filepath.Join(configDirectory, "config.yaml"): "provider: deepseek\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(cliDirectory)

	directories := runtimeConfigDirectories()
	if len(directories) != 0 {
		t.Fatalf("directories = %v; a checkout beside the process must name nothing", directories)
	}
}
