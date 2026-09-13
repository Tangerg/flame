package toolset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestApplyPatchPublishedExamplesExecuteThroughGuards(t *testing.T) {
	root := t.TempDir()
	tools, err := buildCWDTools(root, nil, newReadTracker(), newPathLocker())
	if err != nil {
		t.Fatal(err)
	}
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
