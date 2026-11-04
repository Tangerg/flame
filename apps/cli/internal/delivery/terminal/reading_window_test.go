package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/kit"
)

func newReadingTestConversation(t *testing.T) *transcriptView {
	t.Helper()
	view := newTranscriptView(kit.Dark(), kit.Unicode(), "", input.Wheel{}, highlight.New("github-dark"), 24, false, nil)
	t.Cleanup(view.Close)
	return view
}

func TestReadingWindowKeepsLogicalSelectionAcrossUnrelatedProjection(t *testing.T) {
	view := newReadingTestConversation(t)
	view.Append(&kit.Entry{Theme: view.theme, Body: "keep this selection"})
	root := newSceneTestRoot(t, view.Element())
	surface := grid.NewSurface(32, 8)
	root.Draw(surface.View())
	layout := view.content.Layout()
	var selected base.TextRange
	found := false
	for row, line := range layout.Rows(0, layout.Height()) {
		if strings.Contains(line.Text, "keep this") {
			first, ok := layout.Hit(base.Point{Row: row, Col: line.Offset})
			last, endOK := layout.Hit(base.Point{Row: row, Col: line.Offset + 3})
			if !ok || !endOK {
				t.Fatal("text selection did not resolve through the native layout")
			}
			selected = base.TextRange{Start: first.Start, End: last.End}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("reading window omitted retained text")
	}
	view.selection.Set(selected)
	before, err := view.selection.Text(layout)
	if err != nil || before != "keep" {
		t.Fatalf("initial selection=%q, %v", before, err)
	}
	old := view.content
	view.Append(&kit.Entry{Theme: view.theme, Body: "an unrelated final block"})
	if old.Len() != 0 {
		t.Fatal("superseded native window retained its content references")
	}
	root.Draw(surface.View())
	view.restoreSelection()
	after, err := view.selection.Text(view.content.Layout())
	if err != nil || after != before {
		t.Fatalf("selection after native window replacement=%q, %v; want %q", after, err, before)
	}
	first := view.firstEntry()
	view.entries[first].content = &kit.Entry{Theme: view.theme, Body: "the selected semantic content changed"}
	view.projectEntry(first)
	root.Draw(surface.View())
	view.restoreSelection()
	if view.selection.Active() {
		t.Fatal("changed semantic content retained the old logical selection")
	}
}

func TestReadingWindowPreparesNativeStreamsAndAuthoritativeCompletion(t *testing.T) {
	view := newReadingTestConversation(t)
	root := newSceneTestRoot(t, view.Element())
	surface := grid.NewSurface(36, 10)
	started := conversation.Block{ID: "answer", Kind: conversation.BlockAssistant}
	if err := view.Apply(conversation.BlockStarted{Block: started}); err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(conversation.BlockDelta{BlockID: started.ID, Text: "provisional text"}); err != nil {
		t.Fatal(err)
	}
	live := view.textStreams[conversation.BlockKey("", started.ID)]
	if live.document.Snapshot().Text() != "provisional text" || live.document.Snapshot().Final() {
		t.Fatal("native stream source did not own the open revision")
	}
	root.Draw(surface.View())
	previous := view.content.Layout().Block(view.entries[live.id].nativeID)
	started.Text = "authoritative replacement"
	if err := view.Apply(conversation.BlockCompleted{Block: started}); err != nil {
		t.Fatal(err)
	}
	if !live.document.Snapshot().Final() {
		t.Fatal("stream completion did not close the native source")
	}
	root.Draw(surface.View())
	if got := strings.Join(surface.Rows(), "\n"); !strings.Contains(got, "authoritative replacement") || strings.Contains(got, "provisional text") {
		t.Fatalf("final reading window:\n%s", got)
	}
	old := copyableRowsText(previous.(base.TextProjector).Rows(36))
	if !strings.Contains(old, "provisional text") {
		t.Fatal("authoritative completion mutated a previously prepared passive value")
	}
}

func TestReadingWindowAbortsCandidateGeometryAndRetainsBusinessIdentity(t *testing.T) {
	view := newReadingTestConversation(t)
	for _, body := range []string{"first", "second", "third", "fourth", "fifth", "sixth"} {
		view.Append(&kit.Entry{Theme: view.theme, Body: body})
	}
	root := newSceneTestRoot(t, view.Element())
	root.Draw(grid.NewSurface(32, 6).View())
	previousWidth := view.content.Width()
	candidate, err := root.Prepare(grid.NewSurface(12, 5).View())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Abort()
	if view.content.Width() != previousWidth || view.projected.Value().Width() != previousWidth {
		t.Fatal("aborted frame published candidate geometry")
	}
	view.retain = 4
	firstRetained := view.order[2]
	view.DiscardExcess()
	root.Draw(grid.NewSurface(32, 6).View())
	if len(view.order) != 4 || view.firstEntry() != firstRetained {
		t.Fatal("window retention changed the surviving business identities")
	}
	if view.content.FirstBlock() != 1 {
		t.Fatal("new native window did not own its own item identities")
	}
	view.selectEntry(firstRetained, true)
	if view.selected != firstRetained {
		t.Fatal("navigation used a transient native item identity")
	}
}

func TestReadingWindowReaderKeepsFullSectionsAndScrollIntent(t *testing.T) {
	reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{document: readerDocument{Title: "long content", Sections: []toolSection{{Style: toolSectionCode, Text: strings.Repeat("retained row\n", 35)}}}})
	root := newSceneTestRoot(t, reader.Element())
	surface := grid.NewSurface(40, 8)
	root.Draw(surface.View())
	if reader.content.Height() < 35 {
		t.Fatal("reader dropped full semantic sections")
	}
	reader.scroll.By(6)
	root.Draw(surface.View())
	offset := reader.scroll.Offset()
	reader.replace(readerDocument{Title: "updated content", Sections: []toolSection{{Style: toolSectionCode, Text: strings.Repeat("retained row\n", 45)}}}, true, false)
	root.Draw(surface.View())
	if reader.scroll.Offset() != offset || reader.scroll.FollowingEnd() {
		t.Fatalf("reading position changed: got=%d following=%t, want=%d", reader.scroll.Offset(), reader.scroll.FollowingEnd(), offset)
	}
}
