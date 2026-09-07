package workspace

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestStructuredDiffValidatesAndRendersEveryRow(t *testing.T) {
	t.Parallel()
	diff := Diff{Files: []FileDiff{{
		Change: Change{Path: "main.go", Status: protocol.FileStatusModified},
		Rows: []protocol.DiffRow{
			{Type: protocol.DiffRowHunk, Text: "@@ -1,2 +1,2 @@"},
			{Type: protocol.DiffRowContext, LeftLine: 1, RightLine: 1, Code: "package main"},
			{Type: protocol.DiffRowDeleted, LeftLine: 2, Code: "var old = true"},
			{Type: protocol.DiffRowAdded, RightLine: 2, Code: "var current = true"},
		},
	}}}
	if err := diff.Validate(); err != nil {
		t.Fatal(err)
	}
	want := "diff -- main.go (modified)\n@@ -1,2 +1,2 @@\n package main\n-var old = true\n+var current = true"
	if got := diff.Text(); got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}

	binary := diff
	binary.Files = append([]FileDiff(nil), diff.Files...)
	binary.Files[0].Binary = true
	if err := binary.Validate(); err == nil {
		t.Fatal("a binary file with text rows was accepted")
	}
}

func TestStructuredDiffOwnsPathUniqueness(t *testing.T) {
	t.Parallel()
	diff := Diff{Files: []FileDiff{
		{Change: Change{Path: "main.go", Status: protocol.FileStatusModified}},
		{Change: Change{Path: "main.go", Status: protocol.FileStatusModified}},
	}}

	if err := diff.Validate(); err == nil || !strings.Contains(err.Error(), `repeats path "main.go"`) {
		t.Fatalf("Validate = %v, want duplicate path error", err)
	}
}

func TestReadRequestRefusesAnAmbiguousLineWindow(t *testing.T) {
	t.Parallel()
	if _, err := NewReadLineRange(0, 10); err == nil {
		t.Fatal("line range accepted an end without a positive start")
	}
}

func TestFileContentOwnsOneCompleteRuntimeWindow(t *testing.T) {
	t.Parallel()

	valid := []FileContent{
		{TotalLines: 1},
		{TotalLines: 3, StartLine: 2, EndLine: 3},
	}
	for _, content := range valid {
		if err := content.Validate(); err != nil {
			t.Errorf("Validate rejected valid content %+v: %v", content, err)
		}
	}

	for _, content := range []FileContent{
		{TotalLines: 3, StartLine: 3, EndLine: 2},
		{TotalLines: 3, StartLine: 2, EndLine: 4},
	} {
		if err := content.Validate(); err == nil {
			t.Errorf("Validate accepted invalid content %+v", content)
		}
	}
}

func TestFileListingOwnsPathUniqueness(t *testing.T) {
	t.Parallel()
	listing := FileListing{Entries: []FileEntry{
		{Path: "main.go", Type: protocol.FileEntryFile},
		{Path: "main.go", Type: protocol.FileEntryFile},
	}}
	if err := listing.Validate(); err == nil || !strings.Contains(err.Error(), "repeats path") {
		t.Fatalf("Validate = %v, want duplicate path error", err)
	}
}
