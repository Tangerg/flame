package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffTreatsHeaderLikeHunkContentAsChangedLines(t *testing.T) {
	for _, newline := range []string{"\n", ""} {
		t.Run(newline, func(t *testing.T) {
			dir := initRepo(t)
			write(t, dir, "a.txt", "-- a/false-old.txt"+newline)
			gitTestCommand(t, dir, "commit", "-am", "header-like content")
			write(t, dir, "a.txt", "++ b/false-new.txt"+newline)
			raw, err := RawDiff(t.Context(), dir, "a.txt", Worktree, testMaxDiffBytes)
			if err != nil {
				t.Fatal(err)
			}
			summary, err := SummarizePatch([]byte(raw.Patch))
			if err != nil {
				t.Fatal(err)
			}
			files, err := testDiff(t.Context(), dir, "a.txt", Worktree)
			if err != nil {
				t.Fatal(err)
			}
			for _, result := range [][]DiffFile{summary, files} {
				if len(result) != 1 || result[0].Path != "a.txt" || result[0].Added != 1 || result[0].Removed != 1 {
					t.Fatalf("header-like content changed patch identity: %+v", result)
				}
			}
		})
	}
}

func TestDiffSelectionTreatsResourcePathsLiterally(t *testing.T) {
	for _, names := range [][2]string{
		{"file[1].txt", "file1.txt"}, {"file*.txt", "file-other.txt"},
		{"file?.txt", "file1.txt"}, {":(glob)*.txt", "other.txt"},
	} {
		for _, tracked := range []bool{true, false} {
			state := "untracked"
			if tracked {
				state = "tracked"
			}
			t.Run(names[0]+"/"+state, func(t *testing.T) {
				dir := initRepo(t)
				if tracked {
					for _, name := range names {
						write(t, dir, name, "before\n")
					}
					gitTestCommand(t, dir, "add", ".")
					gitTestCommand(t, dir, "commit", "-m", "literal paths")
				}
				for _, name := range names {
					write(t, dir, name, "after\n")
				}
				listed, err := ListFiles(t.Context(), dir, names[0], 100)
				if err != nil || len(listed) != 1 || listed[0] != names[0] {
					t.Fatalf("ListFiles = %v, %v", listed, err)
				}
				for _, mode := range []Mode{Worktree, Base} {
					files, err := testDiff(t.Context(), dir, names[0], mode)
					if err != nil {
						t.Fatal(err)
					}
					want := 1
					if !tracked && mode == Base {
						want = 0
					}
					if len(files) != want || want == 1 && files[0].Path != names[0] {
						t.Fatalf("%s selection %q = %+v", mode, names[0], files)
					}
					raw, err := RawDiff(t.Context(), dir, names[0], mode, testMaxDiffBytes)
					if err != nil || strings.Count(raw.Patch, diffFileHeader) != want {
						t.Fatalf("raw %s selection %q = %q, %v", mode, names[0], raw.Patch, err)
					}
				}
			})
		}
	}
}

func TestDiffHunkRangesPreserveMultipleHunksAndWholeFileBudgets(t *testing.T) {
	patch := []byte("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n" +
		"@@ -1,2 +1,3 @@\n--- old\n+++ new\n+extra\n \n" +
		"@@ -8 +9 @@\n-old\n+new\n\\ No newline at end of file\n")
	files, truncated, err := parseUnifiedDiff(patch, 1, 8)
	if err != nil || truncated || len(files) != 1 {
		t.Fatalf("multiple hunks = %+v, truncated=%t, %v", files, truncated, err)
	}
	file := files[0]
	if file.Path != "a.txt" || file.Added != 3 || file.Removed != 2 ||
		file.Rows[6].LeftLine != 8 || file.Rows[7].RightLine != 9 || file.Rows[4].Code != "" {
		t.Fatalf("hunk ranges = %+v", file)
	}
	files, truncated, err = parseUnifiedDiff(patch, 1, 7)
	if err != nil || !truncated || len(files) != 0 {
		t.Fatalf("partial file escaped budget: %+v, truncated=%t, %v", files, truncated, err)
	}
	for _, hunk := range []string{"@@ -1,broken +1 @@\n-old\n+new\n", "@@ -1,2 +1 @@\n-old\n+new"} {
		if _, err := SummarizePatch([]byte("diff --git a/a b/a\n" + hunk)); err == nil {
			t.Fatalf("accepted invalid hunk %q", hunk)
		}
	}
}

func TestLiteralDirectorySelectionIncludesOnlyItsDescendants(t *testing.T) {
	dir := initRepo(t)
	for _, parent := range []string{"dir[1]", "dir1"} {
		if err := os.Mkdir(filepath.Join(dir, parent), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, dir, filepath.Join(parent, "tracked.txt"), "before\n")
	}
	gitTestCommand(t, dir, "add", ".")
	gitTestCommand(t, dir, "commit", "-m", "literal directories")
	for _, parent := range []string{"dir[1]", "dir1"} {
		write(t, dir, filepath.Join(parent, "tracked.txt"), "after\n")
		write(t, dir, filepath.Join(parent, "untracked.txt"), "new\n")
	}
	files, err := testDiff(t.Context(), dir, filepath.Join(dir, "dir[1]"), Worktree)
	if err != nil || len(files) != 2 {
		t.Fatalf("directory diff = %+v, %v", files, err)
	}
	for _, file := range files {
		if !strings.HasPrefix(file.Path, "dir[1]/") {
			t.Fatalf("directory selection escaped into %q", file.Path)
		}
	}
	if _, err := ListFiles(t.Context(), dir, "../", 100); err == nil {
		t.Fatal("accepted a resource outside the workspace")
	}
}
