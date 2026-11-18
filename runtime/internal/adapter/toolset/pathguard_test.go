package toolset

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathGuardApplyPatchChecksAllTargets(t *testing.T) {
	for _, patch := range []string{
		"--- a/ok.txt\n+++ b/ok.txt\n@@ -1 +1 @@\n-old\n+new\n" +
			"--- a/.git/config\n+++ b/.git/config\n@@ -1 +1 @@\n-old\n+new\n",
		"diff --git a/ok.txt b/.git/moved\nsimilarity index 100%\nrename from ok.txt\nrename to .git/moved\n",
		"diff --git a/.git/config b/moved.txt\nsimilarity index 100%\nrename from .git/config\nrename to moved.txt\n",
	} {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"ok.txt", ".git/config"} {
			if err := os.WriteFile(filepath.Join(dir, path), []byte("old\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		executable := withPathGuard(mustApplyPatchTool(t, mustLocalExecutor(t, dir)), mustRoot(t, dir))
		arguments, err := json.Marshal(map[string]string{"patch": patch})
		if err != nil {
			t.Fatal(err)
		}
		out := callRejectedTool(t, t.Context(), executable, string(arguments))
		if !strings.Contains(out, "Refused") {
			t.Fatalf("out = %q, want refusal", out)
		}
		for _, path := range []string{"ok.txt", ".git/config"} {
			body, err := os.ReadFile(filepath.Join(dir, path))
			if err != nil || string(body) != "old\n" {
				t.Fatalf("protected patch changed %s: %q, %v", path, body, err)
			}
		}
		for _, path := range []string{"moved.txt", ".git/moved"} {
			if _, err := os.Stat(filepath.Join(dir, path)); !os.IsNotExist(err) {
				t.Fatalf("protected patch created %s: %v", path, err)
			}
		}
	}
}

// TestGuardMutationPathConfinesWrites verifies the filesystem capability
// boundary independently of Run isolation mode.
func TestGuardMutationPathConfinesWrites(t *testing.T) {
	work := t.TempDir()
	outside := filepath.Join(t.TempDir(), "real-project", "main.go")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		path      string
		wantBlock bool
	}{
		{"relative inside", "pkg/main.go", false},
		{"workspace root file", "main.go", false},
		{"absolute outside", outside, true},
		{"absolute inside", filepath.Join(work, "in.go"), false},
		{"parent traversal", "../real-project/main.go", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refusal, ok := guardMutationPath(mustRoot(t, work), tc.path)
			if tc.wantBlock {
				if ok || !strings.Contains(refusal, "outside this workspace") {
					t.Fatalf("%q: ok=%v refusal=%q, want blocked at the workspace boundary", tc.path, ok, refusal)
				}
			} else if !ok {
				t.Fatalf("%q wrongly blocked: %q", tc.path, refusal)
			}
		})
	}
}
