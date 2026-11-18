package toolset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/keylock"
)

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
	tools, err := openCWDTools(root, nil, newReadTracker(), keylock.NewSet())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tools.close(); err != nil {
			t.Error(err)
		}
	})

	refusal := callRejectedTool(t, t.Context(), tools.edit, editArguments(t, ".git/config", "[core]", "[hijacked]"))
	if !strings.Contains(refusal, "protected") {
		t.Fatalf("refusal = %q, want the protected-directory message", refusal)
	}
	if body, _ := os.ReadFile(config); string(body) != "[core]\n" {
		t.Fatalf(".git/config = %q, want it untouched", body)
	}
}
