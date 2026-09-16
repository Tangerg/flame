package toolset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

// edit joins apply_patch under the same guard stack, so it has to earn the same
// things: the file must have been read, the change must land, and a target the
// path guard protects must be refused rather than applied.
func TestEditExecutesThroughTheMutationGuards(t *testing.T) {
	root := t.TempDir()
	notes := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(notes, []byte("first\nsecond\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools, err := buildCWDTools(root, nil, newReadTracker(), newPathLocker())
	if err != nil {
		t.Fatal(err)
	}

	// Unread file: the staleness guard owns this, and it answers the model
	// rather than failing the call.
	refusal, err := callTextTool(t.Context(), tools.edit, editArguments(t, "notes.txt", "first", "FIRST"))
	if err != nil {
		t.Fatalf("editing an unread file: %v", err)
	}
	if !strings.Contains(strings.ToLower(refusal), "read") {
		t.Fatalf("unread-file refusal = %q, want it to say the file must be read first", refusal)
	}
	if body, _ := os.ReadFile(notes); string(body) != "first\nsecond\n" {
		t.Fatalf("refused edit still changed the file: %q", body)
	}

	if _, err := callTextTool(t.Context(), readTool(t, tools), readArguments("notes.txt")); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := callTextTool(t.Context(), tools.edit, editArguments(t, "notes.txt", "first", "FIRST")); err != nil {
		t.Fatalf("edit after read: %v", err)
	}
	body, err := os.ReadFile(notes)
	if err != nil || string(body) != "FIRST\nsecond\n" {
		t.Fatalf("file = %q (%v), want the replacement applied", body, err)
	}
}

// The protected-directory barrier is the one invariant that does not depend on
// approval mode, so a second mutation vocabulary must not open a way around it.
func TestEditRefusesProtectedDirectories(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, ".git", "config")
	if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("[core]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools, err := buildCWDTools(root, nil, newReadTracker(), newPathLocker())
	if err != nil {
		t.Fatal(err)
	}

	refusal, err := callTextTool(t.Context(), tools.edit, editArguments(t, ".git/config", "[core]", "[hijacked]"))
	if err != nil {
		t.Fatalf("editing inside .git: %v", err)
	}
	if !strings.Contains(refusal, "protected") {
		t.Fatalf("refusal = %q, want the protected-directory message", refusal)
	}
	if body, _ := os.ReadFile(config); string(body) != "[core]\n" {
		t.Fatalf(".git/config = %q, want it untouched", body)
	}
}

func readTool(t *testing.T, tools cwdTools) toolcontract.Tool {
	t.Helper()
	for _, candidate := range tools.readSearch {
		if candidate.Definition().Name == "read" {
			return candidate
		}
	}
	t.Fatal("read tool not built")
	return nil
}
