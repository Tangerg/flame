package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/highlight"
)

func TestWorkspaceDiffReaderPreservesFileIdentityAndRows(t *testing.T) {
	diff := workspace.Diff{
		Baseline: protocol.DiffBaseline{Type: protocol.DiffBaselineEmptyTree}, Truncated: true,
		Files: []workspace.FileDiff{
			{Change: workspace.Change{Path: "one.txt", Status: protocol.FileStatusRenamed, PreviousPath: "old.txt"}, Rows: []protocol.DiffRow{
				{Type: protocol.DiffRowHunk, Text: "@@ -10 +20 @@ function"},
				{Type: protocol.DiffRowDeleted, LeftLine: 10, Code: new("old one"), NoNewline: true},
				{Type: protocol.DiffRowAdded, RightLine: 20, Code: new("new one"), NoNewline: true},
			}},
			{Change: workspace.Change{Path: "two.txt", Status: protocol.FileStatusDeleted}, Rows: []protocol.DiffRow{
				{Type: protocol.DiffRowHunk, Text: "@@ -1 +0,0 @@"},
				{Type: protocol.DiffRowDeleted, LeftLine: 1, Code: new("two content")},
			}},
			{Change: workspace.Change{Path: "image.png", Status: protocol.FileStatusModified, Binary: true}},
			{Change: workspace.Change{Path: "empty.txt", Status: protocol.FileStatusAdded}},
		},
	}
	document := workspaceDiffDocument(workspace.DiffRequest{Mode: protocol.DiffModeWorktree, Format: protocol.DiffFormatRows}, diff)
	if !strings.Contains(document.Detail, "truncated") {
		t.Fatal("reader lost truncation feedback")
	}
	clipboard := new(recordingClipboard)
	reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, clipboard)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{document: document})
	root := headless.NewRoot(reader)
	surface := grid.NewSurface(100, 40)
	root.Draw(surface.View())
	reader.copy()
	want := []string{
		`"old.txt" → "one.txt" · renamed`, "@@ -10 +20 @@ function", "10    -old one", "   20 +new one",
		`\ No newline at end of file`, `"two.txt" · deleted`, "@@ -1 +0,0 @@", "1   -two content",
		`"image.png" · modified · binary`, "Binary file.", `"empty.txt" · added`, "No text changes.",
	}
	previous := -1
	for _, value := range want {
		drawnTextOrigin(t, surface, value)
		index := strings.Index(clipboard.text[previous+1:], value)
		if index < 0 {
			t.Fatalf("reader lost or reordered %q in %q", value, clipboard.text)
		}
		previous += index + 1
	}
	if strings.Count(clipboard.text, `\ No newline at end of file`) != 2 {
		t.Fatalf("line-termination markers = %q", clipboard.text)
	}
}

func TestWorkspaceDiffPreservesRawMaterialAndEmptyTruncation(t *testing.T) {
	patch := "diff --git a/one b/one\n--- a/one\n+++ b/one\n@@ -1 +1 @@\n-old\n+new\ndiff --git a/two b/two\nnew file mode 100644\n"
	diff := workspace.Diff{Baseline: protocol.DiffBaseline{Type: protocol.DiffBaselineEmptyTree}, Patch: patch}
	document := workspaceDiffDocument(workspace.DiffRequest{Format: protocol.DiffFormatRaw}, diff)
	if len(document.Sections) != 2 || document.Sections[1].Text != patch || document.Sections[1].Style != toolSectionCode {
		t.Fatalf("raw patch was reinterpreted: %+v", document)
	}
	diff.Patch, diff.Truncated = "", true
	document = workspaceDiffDocument(workspace.DiffRequest{Format: protocol.DiffFormatRows}, diff)
	if document.Sections[1].Text != "No complete file fits the requested row limit." {
		t.Fatalf("truncated diff reported no changes: %+v", document)
	}
}
