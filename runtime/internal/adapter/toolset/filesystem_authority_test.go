package toolset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

func TestFilesystemPoliciesShareScopeDirectoryAuthority(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "data.json"), []byte(`{"name":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".git", filepath.Join(workspace, "alias")); err != nil {
		t.Fatal(err)
	}
	tools, err := openCWDTools(workspace, nil, newReadTracker(), keylock.NewSet())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tools.close(); err != nil {
			t.Error(err)
		}
	})
	direct, err := openDirectTools(workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := direct.Close(); err != nil {
			t.Error(err)
		}
	})
	detached := filepath.Join(parent, "detached")
	if err := os.Rename(workspace, detached); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "alias"), 0o700); err != nil {
		t.Fatal(err)
	}
	replacement := `{"name":"replacement"}`
	if err := os.WriteFile(filepath.Join(workspace, "data.json"), []byte(replacement), 0o600); err != nil {
		t.Fatal(err)
	}

	read, err := callTextTool(t.Context(), tools.readSearch[0], `{"path":"data.json"}`)
	if err != nil || !strings.Contains(read, `old`) || strings.Contains(read, "replacement") {
		t.Fatalf("read = %q, %v", read, err)
	}
	for _, change := range [][2]string{{"old", "new"}, {"new", "latest"}} {
		if _, err := callTextTool(t.Context(), tools.edit, editArguments(t, "data.json", change[0], change[1])); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(filepath.Join(detached, "data.json"))
	if err != nil || string(got) != "{\n  \"name\": \"latest\"\n}\n" {
		t.Fatalf("original directory = %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(workspace, "data.json"))
	if err != nil || string(got) != replacement {
		t.Fatalf("replacement directory was touched: %q, %v", got, err)
	}
	refusal := callRejectedTool(t, t.Context(), tools.edit, editArguments(t, "alias/config", "old", "new"))
	if !strings.Contains(refusal, "protected") {
		t.Fatalf("protected directory alias = %q", refusal)
	}
	for _, query := range []struct {
		tool      toolcontract.Tool
		arguments string
	}{
		{tools.readSearch[1], `{"pattern":"**/*"}`},
		{tools.readSearch[2], `{"pattern":"replacement"}`},
		{direct.Visible[0], `{"path":"data.json"}`},
	} {
		_, err := callTextTool(t.Context(), query.tool, query.arguments)
		var failure *toolcontract.Failure
		if !errors.As(err, &failure) {
			t.Fatalf("detached path-based query error = %v", err)
		}
		text, _ := failure.Output().Text()
		if !strings.Contains(text, "workspace path changed") {
			t.Fatalf("detached query = %q", text)
		}
	}
}

func TestMutationRecordingPreservesScopePartialEffects(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires directory write permissions")
	}
	for _, move := range []bool{false, true} {
		name := "partial patch"
		if move {
			name = "interrupted move"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			locked := filepath.Join(dir, "locked")
			if err := os.Mkdir(locked, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(locked, "original"), []byte("old\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			tools, err := openCWDTools(dir, nil, newReadTracker(), keylock.NewSet())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := tools.close(); err != nil {
					t.Error(err)
				}
			})
			if _, err := callTextTool(t.Context(), tools.readSearch[0], `{"path":"locked/original"}`); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(locked, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(locked, 0o700); err != nil {
					t.Error(err)
				}
			})
			patch := "diff --git a/created b/created\nnew file mode 100644\n--- /dev/null\n+++ b/created\n@@ -0,0 +1,1 @@\n+first\ndiff --git a/locked/original b/locked/original\n--- a/locked/original\n+++ b/locked/original\n@@ -1,1 +1,1 @@\n-old\n+new\n"
			if move {
				patch = "diff --git a/locked/original b/created\nsimilarity index 100%\nrename from locked/original\nrename to created\n"
			}
			arguments, err := json.Marshal(fs.ApplyPatchRequest{Patch: patch})
			if err != nil {
				t.Fatal(err)
			}
			var recorded []string
			ctx := WithMutationRecorder(t.Context(), func(paths []string) { recorded = append(recorded, paths...) })
			_, err = callTextTool(ctx, tools.applyPatch, string(arguments))
			var failure *toolcontract.Failure
			if !errors.As(err, &failure) {
				t.Fatalf("partial patch error = %v", err)
			}
			var response fs.ApplyPatchResponse
			if err := json.Unmarshal(failure.Output().Details, &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Files) != 1 || response.Files[0].Path != "created" || !response.Files[0].Created || response.Files[0].MovedFrom != "" {
				t.Fatalf("Scope partial response = %+v", response)
			}
			if !reflect.DeepEqual(recorded, []string{"created"}) {
				t.Fatalf("recorded = %v, want Scope's acknowledged destination only", recorded)
			}
			if got, err := os.ReadFile(filepath.Join(locked, "original")); err != nil || string(got) != "old\n" {
				t.Fatalf("source changed: %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "created")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAutoFormatUsesScopeTextFormatPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte("\ufeff{\"x\":1}\r\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := formatTestPath(t, path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "\ufeff{\r\n  \"x\": 1\r\n}\r\n" {
		t.Fatalf("formatted = %q, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, %v", info, err)
	}
}

func TestProtectedWorkspaceAliasRetainsPolicyAfterReplacement(t *testing.T) {
	parent := t.TempDir()
	metadata := filepath.Join(parent, ".git")
	if err := os.Mkdir(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "workspace")
	if err := os.Symlink(metadata, alias); err != nil {
		t.Fatal(err)
	}
	root := mustRoot(t, alias)
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(alias, 0o700); err != nil {
		t.Fatal(err)
	}
	if refusal, allowed := guardMutationPath(root, "config"); allowed || !strings.Contains(refusal, "protected") {
		t.Fatalf("metadata root alias: allowed=%t, refusal=%q", allowed, refusal)
	}
}

func TestFilesystemPathsDoNotUseAmbientHome(t *testing.T) {
	root := mustRoot(t, t.TempDir())
	for _, path := range []string{"~", "~/file"} {
		if _, err := resolveRootPath(root, path); err == nil {
			t.Fatalf("accepted ambient home path %q", path)
		}
	}
}
