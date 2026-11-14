package git

import (
	"bytes"
	"strings"
	"testing"
)

func TestVisualProjectionRetainsExactlyTheAcceptedWholeFileBytes(t *testing.T) {
	first := "diff --git a/old.txt b/中文.txt\nrename from old.txt\nrename to 中文.txt\n" +
		"--- a/old.txt\n+++ b/中文.txt\n@@ -1 +1 @@\n-old\n+新🙂\n\\ No newline at end of file\n"
	second := "diff --git a/image.bin b/image.bin\nBinary files a/image.bin and b/image.bin differ\n"
	third := "diff --git a/large.txt b/large.txt\n--- a/large.txt\n+++ b/large.txt\n@@ -0,0 +1,3 @@\n+1\n+2\n+3\n"
	patch := []byte(first + second + third)
	for _, retainRows := range []bool{false, true} {
		parsed, err := parseUnifiedDiff(patch, 5, 3, retainRows)
		if err != nil {
			t.Fatal(err)
		}
		if !parsed.truncated || len(parsed.files) != 2 || !bytes.Equal(patch[:parsed.end], []byte(first+second)) {
			t.Fatalf("whole-file projection = %+v", parsed)
		}
		file := parsed.files[0]
		if file.Path != "中文.txt" || file.Status != StatusRenamed || file.PreviousPath != "old.txt" || file.Added != 1 || file.Removed != 1 {
			t.Fatalf("manifest lost canonical metadata: %+v", file)
		}
		if !parsed.files[1].Binary || parsed.files[1].Path != "image.bin" {
			t.Fatalf("binary manifest = %+v", parsed.files[1])
		}
		if retainRows && (len(file.Rows) != 3 || !file.Rows[2].NoNewline) || !retainRows && file.Rows != nil {
			t.Fatalf("row projection = %+v", file.Rows)
		}
	}
}

func TestVisualProjectionDistinguishesEmptyFromAnOversizedFirstFile(t *testing.T) {
	for _, test := range []struct {
		name      string
		patch     string
		truncated bool
	}{
		{name: "clean"},
		{name: "no complete file fits", patch: "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-old\n+new\n", truncated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseUnifiedDiff([]byte(test.patch), 1, 1, false)
			if err != nil || parsed.truncated != test.truncated || len(parsed.files) != 0 || parsed.end != 0 {
				t.Fatalf("empty projection = %+v, %v", parsed, err)
			}
		})
	}
}

func TestVisualDiffKeepsUntrackedStatusAndRawBytesFromItsOwnRead(t *testing.T) {
	dir := initRepo(t)
	write(t, dir, "new.txt", "新🙂")
	visual, err := Diff(t.Context(), dir, "new.txt", Worktree, Visual, 10, 10, testMaxDiffBytes)
	if err != nil {
		t.Fatal(err)
	}
	if visual.Truncated || len(visual.Files) != 1 || visual.Files[0].Status != StatusUntracked || visual.Files[0].Rows != nil || !strings.Contains(visual.Patch, "\\ No newline at end of file\n") {
		t.Fatalf("visual untracked read = %+v", visual)
	}
	summary, err := SummarizePatch([]byte(visual.Patch))
	if err != nil || len(summary) != 1 || summary[0].Path != visual.Files[0].Path || summary[0].Added != visual.Files[0].Added {
		t.Fatalf("patch and manifest disagree: %+v, %v", summary, err)
	}
}
