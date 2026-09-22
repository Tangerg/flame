package toolset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestApplyPatchPublishedExamplesExecuteThroughGuards(t *testing.T) {
	root := t.TempDir()
	tools, err := openCWDTools(root, nil, newReadTracker(), keylock.NewSet())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tools.close(); err != nil {
			t.Error(err)
		}
	})
	examples := regexp.MustCompile("(?s)```diff\\n(.*?)```").FindAllStringSubmatch(tools.applyPatch.Definition().Description, -1)
	if len(examples) != 3 {
		t.Fatal("apply_patch must publish executable create, modify, and delete examples")
	}
	for i, example := range examples {
		if i > 0 {
			for _, reader := range tools.readSearch {
				if reader.Definition().Name == "read" {
					if _, err := callTextTool(t.Context(), reader, `{"path":"notes.txt"}`); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		arguments, err := json.Marshal(map[string]string{"patch": example[1]})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := callTextTool(t.Context(), tools.applyPatch, string(arguments)); err != nil {
			t.Fatalf("published example %d: %v", i+1, err)
		}
		if i < 2 {
			body, err := os.ReadFile(filepath.Join(root, "notes.txt"))
			want := []string{"first\n", "second\n"}[i]
			if err != nil || string(body) != want {
				t.Fatalf("example %d content = %q, %v", i+1, body, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("delete example left file: %v", err)
	}
}

func TestApplyPatchNativePathsSurviveGuards(t *testing.T) {
	root := t.TempDir()
	tools, err := openCWDTools(root, nil, newReadTracker(), keylock.NewSet())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tools.close(); err != nil {
			t.Error(err)
		}
	})
	patch := strings.ReplaceAll("--- /dev/null\n+++ b/notes.txt\n@@ -0,0 +1 @@\n+created\n", "\n", "\r\n")
	arguments, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	reporter, found, err := toolcontract.Capability[FileMutationReporter](tools.applyPatch)
	if err != nil || !found {
		t.Fatalf("native patch capability unavailable: %v, %v", found, err)
	}
	paths, err := reporter.MutationPaths(arguments)
	if err != nil || !slices.Equal(paths, []string{"notes.txt"}) {
		t.Fatalf("CRLF patch paths = %v, %v", paths, err)
	}
	var recorded []string
	ctx := WithMutationRecorder(t.Context(), func(paths []string) { recorded = append(recorded, paths...) })
	if _, err := callTextTool(ctx, tools.applyPatch, string(arguments)); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil || string(body) != "created\n" {
		t.Fatalf("guarded patch content = %q, %v", body, err)
	}
	if !slices.Equal(recorded, []string{"notes.txt"}) {
		t.Fatalf("acknowledged mutations = %v", recorded)
	}
}
