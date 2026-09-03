package workspace

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestChangeOwnsItsRenameAndBinaryInvariants(t *testing.T) {
	t.Parallel()
	added, removed := 3, 2
	tests := []struct {
		name   string
		change Change
		want   string
	}{
		{name: "text", change: Change{Path: "main.go", Status: protocol.FileStatusModified, Added: &added, Removed: &removed}},
		{name: "rename", change: Change{Path: "new.go", PreviousPath: "old.go", Status: protocol.FileStatusRenamed}},
		{name: "rename missing source", change: Change{Path: "new.go", Status: protocol.FileStatusRenamed}, want: "previousPath"},
		{name: "source on modification", change: Change{Path: "new.go", PreviousPath: "old.go", Status: protocol.FileStatusModified}, want: "previousPath"},
		{name: "binary counts", change: Change{Path: "logo.png", Status: protocol.FileStatusModified, Binary: true, Added: &added}, want: "added"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.change.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWorkspaceOwnsResolvedIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace Workspace
		want      string
	}{
		{name: "available", workspace: Workspace{Path: "/repo/work", ProjectRoot: "/repo", Availability: protocol.WorkspaceAvailable}},
		{name: "missing", workspace: Workspace{Path: "/gone/work", ProjectRoot: "/gone", Availability: protocol.WorkspaceMissing}},
		{name: "relative path", workspace: Workspace{Path: "work", ProjectRoot: "/repo", Availability: protocol.WorkspaceAvailable}, want: "not absolute"},
		{name: "empty project root", workspace: Workspace{Path: "/repo", Availability: protocol.WorkspaceAvailable}, want: "project root is empty"},
		{name: "relative project root", workspace: Workspace{Path: "/repo/work", ProjectRoot: "repo", Availability: protocol.WorkspaceAvailable}, want: "project root is not absolute"},
		{name: "unknown availability", workspace: Workspace{Path: "/repo", ProjectRoot: "/repo", Availability: protocol.WorkspaceAvailability("unknown")}, want: "availability"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.workspace.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStructuredDiffValidatesAndRendersEveryRow(t *testing.T) {
	t.Parallel()
	diff := Diff{Files: []FileDiff{{
		Change: Change{Path: "main.go", Status: protocol.FileStatusModified},
		Rows: []DiffRow{
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

	invalid := diff
	invalid.Files = append([]FileDiff(nil), diff.Files...)
	invalid.Files[0].Rows = append([]DiffRow(nil), diff.Files[0].Rows...)
	invalid.Files[0].Rows[1].LeftLine = 0
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid context row was accepted")
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
		{Path: "empty.txt", TotalLines: 1},
		{Path: "window.txt", TotalLines: 3, StartLine: 2, EndLine: 3},
	}
	for _, content := range valid {
		if err := content.Validate(); err != nil {
			t.Errorf("Validate rejected valid content %+v: %v", content, err)
		}
	}

	for _, content := range []FileContent{
		{Path: "empty.txt"},
		{Path: "window.txt", TotalLines: 3, StartLine: 2},
		{Path: "window.txt", TotalLines: 3, StartLine: 2, EndLine: 4},
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
