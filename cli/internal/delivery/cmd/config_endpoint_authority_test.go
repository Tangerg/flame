package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectConfigurationCannotSelectCredentialDestination(t *testing.T) {
	t.Setenv("FLAME_CLI_RUNTIME_ENDPOINT", "")
	t.Setenv("FLAME_RUNTIME_TOKEN", "synthetic-runtime-token")
	for name, content := range map[string]string{
		"nested":      "runtime:\n  endpoint: https://project-selected.invalid\n",
		"case folded": "Runtime:\n  Endpoint: https://project-selected.invalid\n",
	} {
		t.Run(name, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, ".flame.yaml")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := executeCommand(t, instantRuntime(), "", "-C", workspace, "sessions", "ls")
			if err == nil || !strings.Contains(err.Error(), "project configuration cannot select") {
				t.Fatalf("project-selected credential destination was admitted: %v", err)
			}
			if strings.Contains(err.Error(), "synthetic-runtime-token") {
				t.Fatal("configuration refusal exposed a credential")
			}
			// Explicit user selection, not the file's basename, establishes authority.
			out, _, err := executeCommand(t, instantRuntime(), "", "-C", workspace, "--config", path, "config", "show")
			if err != nil || !strings.Contains(out, "https://project-selected.invalid") {
				t.Fatalf("explicit configuration was rejected: %q, %v", out, err)
			}
		})
	}
}

func TestProjectPreferencesStillAllowExplicitEndpointSelection(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".flame.yaml"), []byte("ui:\n  mouse: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_CLI_RUNTIME_ENDPOINT", "https://user-selected.invalid")
	out, _, err := executeCommand(t, instantRuntime(), "", "-C", workspace, "config", "show")
	if err != nil || !strings.Contains(out, "https://user-selected.invalid") {
		t.Fatalf("explicit environment endpoint failed: %q, %v", out, err)
	}
}

func TestDynamicCompletionRejectsProjectSelectedEndpoint(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".flame.yaml"), []byte("runtime:\n  endpoint: https://project-selected.invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRoot(Dependencies{OpenRuntime: func(context.Context, string) (Runtime, RuntimeProfile, error) {
		t.Fatal("project configuration selected a Runtime during completion")
		return nil, nil, nil
	}})
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"__complete", "-C", workspace, "sessions", "show", ""})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "ShellCompDirectiveError") {
		t.Fatalf("completion did not reject project endpoint: %q", output.String())
	}
}
