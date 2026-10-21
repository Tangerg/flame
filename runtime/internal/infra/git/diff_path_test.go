package git

import "testing"

func TestParseUnifiedDiffRecoversBinaryFilePath(t *testing.T) {
	patch := []byte("diff --git a/image.bin b/image.bin\n" +
		"Binary files a/image.bin and b/image.bin differ\n")

	files, truncated, err := parseUnifiedDiff(patch, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || truncated || !files[0].Binary || files[0].Path != "image.bin" {
		t.Fatalf("binary diff = %+v, truncated=%v; want image.bin", files, truncated)
	}
}

func TestParseUnifiedDiffDecodesGitQuotedPath(t *testing.T) {
	patch := []byte("diff --git \"a/odd\\tname.txt\" \"b/odd\\tname.txt\"\n" +
		"--- \"a/odd\\tname.txt\"\n" +
		"+++ \"b/odd\\tname.txt\"\n" +
		"@@ -1 +1 @@\n" +
		"-before\n" +
		"+after\n")

	files, truncated, err := parseUnifiedDiff(patch, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || truncated || files[0].Path != "odd\tname.txt" {
		t.Fatalf("quoted diff = %+v, truncated=%v; want decoded path", files, truncated)
	}
}

func TestSummarizePatchCountsEachFileWithoutRows(t *testing.T) {
	patch := []byte("diff --git a/new.txt b/new.txt\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/new.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+one\n" +
		"+two\n" +
		"diff --git a/old.txt b/moved.txt\n" +
		"rename from old.txt\n" +
		"rename to moved.txt\n" +
		"--- a/old.txt\n" +
		"+++ b/moved.txt\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-was\n" +
		"+now\n")

	files, err := SummarizePatch(patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("summary files = %d, want 2", len(files))
	}
	created := files[0]
	if created.Path != "new.txt" || created.Status != StatusAdded ||
		created.Added != 2 || created.Removed != 0 || created.Rows != nil {
		t.Fatalf("created summary = %+v", created)
	}
	moved := files[1]
	if moved.Path != "moved.txt" || moved.PreviousPath != "old.txt" || moved.Status != StatusRenamed ||
		moved.Added != 1 || moved.Removed != 1 || moved.Rows != nil {
		t.Fatalf("moved summary = %+v", moved)
	}
}

func TestSummarizePatchDecodesQuotedPathTheNaiveParserMisses(t *testing.T) {
	patch := []byte("diff --git \"a/odd\\tname.txt\" \"b/odd\\tname.txt\"\n" +
		"--- \"a/odd\\tname.txt\"\n" +
		"+++ \"b/odd\\tname.txt\"\n" +
		"@@ -1 +1 @@\n" +
		"-before\n" +
		"+after\n")

	files, err := SummarizePatch(patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "odd\tname.txt" || files[0].Added != 1 || files[0].Removed != 1 {
		t.Fatalf("quoted summary = %+v", files)
	}
}
