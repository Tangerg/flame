package git

import "testing"

func TestParseUnifiedDiffStopsBeforeAFileBeyondTheCountBudget(t *testing.T) {
	patch := []byte("diff --git a/a.bin b/a.bin\n" +
		"Binary files a/a.bin and b/a.bin differ\n" +
		"diff --git a/b.txt b/b.txt\n" +
		"--- a/b.txt\n" +
		"+++ b/b.txt\n" +
		"@@ malformed\n")

	files, truncated, err := parseUnifiedDiff(patch, 1, 5)
	if err != nil {
		t.Fatalf("parse beyond complete-file budget: %v", err)
	}
	if len(files) != 1 || !truncated {
		t.Fatalf("parse = %d files, truncated=%v; want one file and an immediate cut", len(files), truncated)
	}
}

func TestParseUnifiedDiffStopsBeforeAFileBeyondTheRowBudget(t *testing.T) {
	patch := []byte("diff --git a/a.txt b/a.txt\n" +
		"--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -0,0 +1,3 @@\n" +
		"+one\n" +
		"+two\n" +
		"+three\n")

	files, truncated, err := parseUnifiedDiff(patch, 1, 3)
	if err != nil {
		t.Fatalf("parse beyond complete-file row budget: %v", err)
	}
	if len(files) != 0 || !truncated {
		t.Fatalf("parse = %d files, truncated=%v; want zero whole files and an honest cut", len(files), truncated)
	}
}
