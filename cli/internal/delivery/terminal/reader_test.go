package terminal

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/highlight"
)

func TestReaderKeepsToolContentThatTheInlineBlockSummarizes(t *testing.T) {
	lines := make([]string, maxToolDetailLines+60)
	for i := range lines {
		lines[i] = fmt.Sprintf("contract line %03d", i+1)
	}
	call := conversation.ToolCall{
		Kind: conversation.ToolShell, Command: "long output", Status: conversation.ToolOK,
		Output: strings.Join(lines, "\n"),
	}
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	tool := newToolBlock(presentation, conversation.Block{Kind: conversation.BlockTool, Tool: &call})
	tool.SetExpanded(true)
	inline := drawToolBlock(tool, 72)
	if !strings.Contains(inline, "lines omitted") {
		t.Fatal("inline tool did not retain its bounded summary contract")
	}

	reader := newReaderPane(presentation.Theme, presentation.Glyphs, presentation.Syntax, input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{source: tool})
	root := headless.NewRoot(reader)
	root.Draw(grid.NewSurface(72, 16).View())
	full := copyableRowsText(reader.content.Rows(reader.content.StartRow(), reader.content.Height()))
	for _, want := range []string{lines[0], lines[179], lines[len(lines)-1]} {
		if !strings.Contains(full, want) {
			t.Errorf("reader does not contain %q", want)
		}
	}
	if strings.Contains(full, "lines omitted") {
		t.Fatal("reader reused the inline truncation policy")
	}
}

func TestReaderLiveTailFollowsOnlyAfterTheReaderMovesToTheBottom(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	call := conversation.ToolCall{
		Kind: conversation.ToolShell, Command: "stream", Status: conversation.ToolRunning,
		Output: strings.Repeat("initial output row\n", 40),
	}
	tool := newToolBlock(presentation, conversation.Block{Kind: conversation.BlockTool, Tool: &call})
	reader := newReaderPane(presentation.Theme, presentation.Glyphs, presentation.Syntax, input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{source: tool})
	root := headless.NewRoot(reader)
	surface := grid.NewSurface(60, 10)
	root.Draw(surface.View())

	reader.scroll.By(5)
	root.Draw(surface.View())
	wantOffset := reader.scroll.Offset()
	tool.AppendOutput(strings.Repeat("new row while reading\n", 10))
	root.Draw(surface.View())
	if reader.scroll.FollowingEnd() || reader.scroll.Offset() != wantOffset {
		t.Fatalf("live update moved reader from offset %d to (%d, bottom=%t)", wantOffset, reader.scroll.Offset(), reader.scroll.FollowingEnd())
	}

	reader.scroll.ToBottom()
	root.Draw(surface.View())
	before := reader.scroll.Offset()
	tool.AppendOutput(strings.Repeat("followed row\n", 10))
	root.Draw(surface.View())
	if !reader.scroll.FollowingEnd() || reader.scroll.Offset() <= before {
		t.Fatalf("live tail = (offset %d, bottom=%t), want an advancing followed end after %d", reader.scroll.Offset(), reader.scroll.FollowingEnd(), before)
	}
}

func TestReaderSearchStepsAcrossFullContent(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	reader := newReaderPane(presentation.Theme, presentation.Glyphs, presentation.Syntax, input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{document: readerDocument{
		Title:    "search contract",
		Sections: []ToolSection{{Style: toolSectionCode, Text: "needle one\nother\nneedle two"}},
	}})
	headless.NewRoot(reader).Draw(grid.NewSurface(60, 10).View())
	reader.Find("needle")

	select {
	case result := <-reader.SearchResults():
		if !reader.AcceptSearch(result) {
			t.Fatal("reader rejected its current search result")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reader search did not finish")
	}
	if len(reader.matches) != 2 || reader.current != 0 {
		t.Fatalf("search state = (%d matches, current %d)", len(reader.matches), reader.current)
	}
	if !reader.StepMatch(1) || reader.current != 1 {
		t.Fatalf("next search match = %d, want 1", reader.current)
	}
}

func TestReaderRejectsAResultFromAnEarlierIdenticalSearch(t *testing.T) {
	reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{document: readerDocument{
		Title: "search ownership", Sections: []ToolSection{{Style: toolSectionCode, Text: "needle"}},
	}})
	headless.NewRoot(reader).Draw(grid.NewSurface(60, 10).View())
	reader.Find("needle")
	var previous headless.Result
	select {
	case previous = <-reader.SearchResults():
	case <-time.After(2 * time.Second):
		t.Fatal("reader search did not finish")
	}
	reader.Find("needle")
	if reader.AcceptSearch(previous) {
		t.Fatal("reader accepted a superseded search with the same query")
	}
	select {
	case current := <-reader.SearchResults():
		if !reader.AcceptSearch(current) || len(reader.matches) != 1 {
			t.Fatal("reader rejected its current search")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("replacement search did not finish")
	}
}

func TestReaderCopiesOnlyAnUninterruptedOwnedSelectionGesture(t *testing.T) {
	clipboard := new(recordingClipboard)
	reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, clipboard)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{document: readerDocument{
		Title:    "pointer ownership",
		Sections: []ToolSection{{Style: toolSectionParagraph, Text: "copy only from the owned gesture"}},
	}})
	root := headless.NewRoot(reader)
	root.Draw(grid.NewSurface(60, 10).View())
	start, end := image.Pt(0, 1), image.Pt(3, 1)
	root.Handle(input.Mouse{Pos: start, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: end, Action: input.MouseDrag, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: end, Action: input.MouseUp, Button: input.ButtonLeft})
	if clipboard.text == "" {
		t.Fatal("owned selection gesture did not copy")
	}

	interruptions := []struct {
		name  string
		begin bool
		run   func()
	}{
		{name: "unowned release", run: func() {}},
		{name: "capture cancellation", begin: true, run: func() {
			reader.Handle(input.Mouse{Pos: end, Action: input.MouseCancel})
		}},
		{name: "keyboard", begin: true, run: func() {
			root.Handle(input.Key{Code: input.F3})
		}},
		{name: "different button", begin: true, run: func() {
			root.Handle(input.Mouse{Pos: end, Action: input.MouseUp, Button: input.ButtonRight})
		}},
	}
	for _, interruption := range interruptions {
		t.Run(interruption.name, func(t *testing.T) {
			clipboard.text = ""
			if interruption.begin {
				root.Handle(input.Mouse{Pos: start, Action: input.MouseDown, Button: input.ButtonLeft})
				root.Handle(input.Mouse{Pos: end, Action: input.MouseDrag, Button: input.ButtonLeft})
			}
			interruption.run()
			root.Handle(input.Mouse{Pos: end, Action: input.MouseUp, Button: input.ButtonLeft})
			if clipboard.text != "" {
				t.Fatalf("interrupted selection copied %q", clipboard.text)
			}
		})
	}
}

type readerDocumentSourceProbe struct {
	released int
}

func (r *readerDocumentSourceProbe) Observe(observer func(readerDocument)) func() {
	observer(readerDocument{Title: "live transcript tool"})
	return func() { r.released++ }
}

func TestSameSessionProjectionReplacementRetiresALiveTranscriptReader(t *testing.T) {
	reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	source := new(readerDocumentSourceProbe)
	reader.Open(readerTarget{source: source})

	operations := newOperationOwner(t.Context())
	t.Cleanup(operations.Close)
	application := &app{
		operations: operations,
		session:    sessionState{current: conversation.Session{ID: "session"}},
		execution:  executionState{conversation: conversation.New()},
		dialogs:    dialogState{reader: reader},
	}
	application.prepareSessionProjectionReplacement(conversation.Session{ID: "session"}, conversation.New())

	if source.released != 1 {
		t.Fatalf("live reader source released %d times, want 1", source.released)
	}
}

func TestSameSessionProjectionReplacementPreservesAStaticReader(t *testing.T) {
	reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{document: readerDocument{Title: "authoritative runtime document"}})

	application := &app{
		session:   sessionState{current: conversation.Session{ID: "session"}},
		execution: executionState{conversation: conversation.New()},
		dialogs:   dialogState{reader: reader, runtimeReader: runtimeReaderGoal},
	}
	application.prepareSessionProjectionReplacement(conversation.Session{ID: "session"}, conversation.New())

	if application.dialogs.runtimeReader != runtimeReaderGoal {
		t.Fatalf("runtime reader mode = %d, want the static reader preserved", application.dialogs.runtimeReader)
	}
}
