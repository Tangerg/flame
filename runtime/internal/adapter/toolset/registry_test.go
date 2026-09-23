package toolset_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestDiagnosticRegistryListsOnlyDirectTools(t *testing.T) {
	registry := newDiagnosticRegistry(t)

	found, err := registry.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantClasses := map[string]tool.SafetyClass{
		"read": tool.SafetyClassSafe,
		"glob": tool.SafetyClassSafe,
		"grep": tool.SafetyClassSafe,
	}
	got := make(map[string]tool.SafetyClass, len(found))
	for _, candidate := range found {
		got[candidate.Name] = candidate.SafetyClass
		if err := candidate.ToolDefinition.Validate(); err != nil {
			t.Errorf("tool %q has invalid Scope definition: %v", candidate.Name, err)
		}
		if candidate.Description == "" {
			t.Errorf("tool %q has empty description", candidate.Name)
		}
	}
	for name, want := range wantClasses {
		if got[name] != want {
			t.Errorf("tool %q safety = %q, want %q", name, got[name], want)
		}
	}
	if len(got) != len(wantClasses) {
		t.Fatalf("direct tool count = %d, want %d (%v)", len(got), len(wantClasses), got)
	}
}

func TestDiagnosticRegistryInvokesWithinRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("flame"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := newDiagnosticRegistry(t)
	output, err := registry.Invoke(t.Context(), root, "read", diagnosticArguments(t, `{"path":"note.txt"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if value := output.Any(); !strings.Contains(value.(map[string]any)["content"].(string), "flame") {
		t.Errorf("Invoke output missing flame: %#v", value)
	}
}

func TestDiagnosticRegistryValidatesBeforeNormalizingArguments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("flame\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		arguments string
	}{
		{"read", `{"Path":"note.txt"}`},
		{"read", `{"path":"note.txt","Start_Line":1}`},
		{"read", `{"path":"note.txt","start_line":0}`},
		{"read", `{"path":"note.txt","max_lines":0}`},
		{"read", `{"path":"note.txt","start_line":null}`},
		{"glob", `{"Pattern":"*.txt"}`},
		{"glob", `{"pattern":"*.txt","max_results":null}`},
		{"grep", `{"Pattern":"flame"}`},
		{"grep", `{"pattern":"flame","max_results":null}`},
	} {
		t.Run(test.name+"/"+test.arguments, func(t *testing.T) {
			_, err := newDiagnosticRegistry(t).Invoke(t.Context(), root, test.name, diagnosticArguments(t, test.arguments))
			if !errors.Is(err, tool.ErrInvalidArguments) {
				t.Fatalf("Invoke(%s) error = %v, want invalid Tool input", test.arguments, err)
			}
		})
	}
}

func TestDiagnosticRegistryRejectsUnknownOrEscapingTool(t *testing.T) {
	registry := newDiagnosticRegistry(t)
	if _, err := registry.Invoke(t.Context(), t.TempDir(), "shell", diagnosticArguments(t, `{}`)); err == nil {
		t.Fatal("Invoke error = nil, want unknown-tool error")
	}
	outside := t.TempDir()
	if _, err := registry.Invoke(t.Context(), outside, "read", diagnosticArguments(t, `{"path":"../escape"}`)); err == nil {
		t.Fatal("Invoke escaping path error = nil")
	}
	if _, err := registry.Invoke(t.Context(), outside, "glob", diagnosticArguments(t, `{"pattern":"../**/*"}`)); err == nil {
		t.Fatal("Invoke escaping glob pattern error = nil")
	}
	if _, err := registry.Invoke(t.Context(), outside, "read", diagnosticArguments(t, `{"path":"safe.txt","file_path":"ignored.txt"}`)); err == nil {
		t.Fatal("Invoke read with removed file_path error = nil")
	}
}

func TestDiagnosticRegistryRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not in workspace"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}

	_, err := newDiagnosticRegistry(t).Invoke(t.Context(), root, "read", diagnosticArguments(t, `{"path":"outside/secret.txt"}`))
	if !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Fatalf("Invoke symlink escape error = %v, want ErrPathOutsideRoot", err)
	}
}

func diagnosticArguments(t *testing.T, raw string) tool.Arguments {
	t.Helper()
	arguments, err := tool.ParseArguments(raw)
	if err != nil {
		t.Fatalf("ParseArguments: %v", err)
	}
	return arguments
}

func newDiagnosticRegistry(t *testing.T) toolset.DiagnosticRegistry {
	t.Helper()
	registry, err := toolset.NewDiagnosticRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestDiagnosticRegistryRequiresExplicitDirectory(t *testing.T) {
	for _, directory := range []string{"", ".", "relative"} {
		if _, err := toolset.NewDiagnosticRegistry(directory); err == nil {
			t.Fatalf("accepted implicit directory %q", directory)
		}
	}
}
