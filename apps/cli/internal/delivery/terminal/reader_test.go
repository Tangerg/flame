package terminal

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/highlight"
)

func TestReaderKeepsToolContentThatTheInlineBlockSummarizes(t *testing.T) {
	lines := make([]string, maxToolDetailLines+60)
	for i := range lines {
		lines[i] = fmt.Sprintf("contract line %03d", i+1)
	}
	call := conversation.ToolCall{
		Kind: builtin.CategoryShell, Command: "long output", Status: conversation.ToolOK,
		Output: strings.Join(lines, "\n"),
	}
	presentation := blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
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
	presentation := blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	call := conversation.ToolCall{
		Kind: builtin.CategoryShell, Command: "stream", Status: conversation.ToolRunning,
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
	presentation := blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	reader := newReaderPane(presentation.Theme, presentation.Glyphs, presentation.Syntax, input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{document: readerDocument{
		Title:    "search contract",
		Sections: []toolSection{{Style: toolSectionCode, Text: "needle one\nother\nneedle two"}},
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
		Title: "search ownership", Sections: []toolSection{{Style: toolSectionCode, Text: "needle"}},
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
		Sections: []toolSection{{Style: toolSectionParagraph, Text: "copy only from the owned gesture"}},
	}})
	root := headless.NewRoot(reader)
	root.Draw(grid.NewSurface(60, 10).View())
	start, end := image.Pt(0, 1), image.Pt(3, 1)
	root.Handle(input.Mouse{Pos: start, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: end, Action: input.MouseDrag, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: end, Action: input.MouseUp, Button: input.ButtonLeft})
	if clipboard.text != "copy" {
		t.Fatalf("owned selection copied %q, want the selected word", clipboard.text)
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

func TestReaderCopiesExactlyTheDisplayedBodySelection(t *testing.T) {
	lines := []string{"alpha first", "beta middle", "gamma last"}
	for _, size := range []image.Point{image.Pt(60, 8), image.Pt(22, 5)} {
		for _, line := range lines {
			t.Run(fmt.Sprintf("%dx%d/%s", size.X, size.Y, line), func(t *testing.T) {
				clipboard := new(recordingClipboard)
				reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, clipboard)
				t.Cleanup(reader.Shutdown)
				reader.Open(readerTarget{document: readerDocument{Title: "Reader heading", Sections: []toolSection{{Style: toolSectionCode, Text: strings.Join(lines, "\n")}}}})
				root := headless.NewRoot(reader)
				surface := grid.NewSurface(size.X, size.Y)
				root.Draw(surface.View())
				start := drawnTextOrigin(t, surface, line)
				end := start.Add(image.Pt(len(line)-1, 0))
				root.Handle(input.Mouse{Pos: start, Action: input.MouseDown, Button: input.ButtonLeft})
				root.Handle(input.Mouse{Pos: end, Action: input.MouseDrag, Button: input.ButtonLeft})
				root.Handle(input.Mouse{Pos: end, Action: input.MouseUp, Button: input.ButtonLeft})
				if clipboard.text != line {
					t.Fatalf("selected %q at %v, copied %q", line, start, clipboard.text)
				}
			})
		}
	}
}

func TestReaderBodyPointerCaptureFollowsCommittedGeometry(t *testing.T) {
	for _, mode := range []string{"outside", "resized", "hidden", "scrolled", "header", "footer"} {
		t.Run(mode, func(t *testing.T) {
			clipboard := new(recordingClipboard)
			reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, clipboard)
			t.Cleanup(reader.Shutdown)
			reader.Open(readerTarget{document: readerDocument{Title: "Reader heading", Sections: []toolSection{{Style: toolSectionCode, Text: "alpha first\nbeta middle\ngamma last"}}}})
			root := headless.NewRoot(reader)
			surface := grid.NewSurface(30, 5)
			root.Draw(surface.View())
			start := drawnTextOrigin(t, surface, "alpha first")
			end := image.Pt(100, 100)
			want := "alpha first\nbeta middle\ngamma last"
			switch mode {
			case "header":
				start, end, want = image.Pt(0, 0), image.Pt(5, 0), ""
			case "footer":
				start, end, want = image.Pt(0, 4), image.Pt(5, 4), ""
			case "scrolled":
				root.Handle(input.Mouse{Pos: start, Action: input.WheelDown})
				root.Draw(surface.View())
				start = drawnTextOrigin(t, surface, "gamma last")
				end, want = start.Add(image.Pt(9, 0)), "gamma last"
			}
			root.Handle(input.Mouse{Pos: start, Action: input.MouseDown, Button: input.ButtonLeft})
			if mode == "resized" {
				root.Draw(grid.NewSurface(22, 4).View())
				want = "alpha first\nbeta middle"
			}
			if mode == "hidden" {
				root.Draw(grid.NewSurface(22, 1).View())
				want = ""
			}
			root.Handle(input.Mouse{Pos: end, Action: input.MouseDrag, Button: input.ButtonLeft})
			root.Handle(input.Mouse{Pos: end, Action: input.MouseUp, Button: input.ButtonLeft})
			if clipboard.text != want {
				t.Fatalf("copied %q, want %q", clipboard.text, want)
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
	lifecycle := &sessionLifecycle{
		state:   sessionState{current: conversation.Session{ID: "session"}},
		runs:    &runs{execution: executionState{conversation: conversation.New()}},
		reader:  &readerSurface{pane: reader, tasks: unattachedTasks(operations)},
		prompts: testInterruptPrompts(t),
	}
	lifecycle.prepareSessionProjectionReplacement(conversation.Session{ID: "session"}, conversation.New())

	if source.released != 1 {
		t.Fatalf("live reader source released %d times, want 1", source.released)
	}
}

func TestSameSessionProjectionReplacementPreservesAStaticReader(t *testing.T) {
	reader := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, nil)
	t.Cleanup(reader.Shutdown)
	reader.Open(readerTarget{document: readerDocument{Title: "authoritative runtime document"}})

	goal := &readerQuery{status: "loading session goal"}
	lifecycle := &sessionLifecycle{
		state:   sessionState{current: conversation.Session{ID: "session"}},
		runs:    &runs{execution: executionState{conversation: conversation.New()}},
		reader:  &readerSurface{pane: reader, shown: goal},
		prompts: testInterruptPrompts(t),
	}
	lifecycle.prepareSessionProjectionReplacement(conversation.Session{ID: "session"}, conversation.New())

	if lifecycle.reader.shown != goal {
		t.Fatal("same-session replacement dismissed a static runtime reader")
	}
}

func testReaderSurface(t *testing.T, operations *operationOwner) *readerSurface {
	t.Helper()
	pane := newReaderPane(kit.Dark(), kit.Unicode(), highlight.New("github-dark"), input.Wheel{}, nil)
	t.Cleanup(pane.Shutdown)
	return &readerSurface{pane: pane, tasks: unattachedTasks(operations)}
}

func TestReaderUpdatesOnlyTheDocumentItsOwnerOpened(t *testing.T) {
	operations := newOperationOwner(t.Context())
	t.Cleanup(operations.Close)
	surface := testReaderSurface(t, operations)
	var stack headless.Stack
	stack.SetBase(testTranscriptView(t))
	surface.dialog = newPresentationDialog(kit.DialogConfig{Stack: &stack, Theme: kit.Dark(), Glyphs: kit.Unicode(), Title: "Reader", Body: surface.pane})

	authorization := &readerQuery{status: "MCP authorization"}
	surface.open(authorization, readerDocument{Title: "authorization pending"})
	surface.update(authorization, readerDocument{Title: "authorization granted"})
	if surface.pane.title != "authorization granted" {
		t.Fatalf("owner update showed %q", surface.pane.title)
	}
	surface.open(nil, readerDocument{Title: "another document"})
	surface.update(authorization, readerDocument{Title: "late authorization"})
	if surface.pane.title != "another document" {
		t.Fatalf("a closed owner replaced the shown document with %q", surface.pane.title)
	}
	surface.reload()
}

func TestReaderQueriesFollowTheirTopicAndItsResync(t *testing.T) {
	follows := followsTopic(protocol.TopicHooksChanged)
	for _, test := range []struct {
		event changefeed.Event
		want  bool
	}{
		{changefeed.Event{Type: protocol.RuntimeHooksChanged}, true},
		{changefeed.Event{Type: protocol.RuntimeResync, Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged, protocol.TopicHooksChanged}}, true},
		{changefeed.Event{Type: protocol.RuntimeSkillsChanged}, false},
		{changefeed.Event{Type: protocol.RuntimeResync, Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged}}, false},
	} {
		if got := follows(test.event); got != test.want {
			t.Errorf("follows(%+v) = %t, want %t", test.event, got, test.want)
		}
	}
}

// testScreenHost presents screens over stack; operations is nil when the test
// starts no background work.
func testScreenHost(t *testing.T, transcript *transcriptView, stack *headless.Stack, operations *operationOwner) screenHost {
	t.Helper()
	status := &statusView{}
	host := screenHost{
		status: status, message: status.note, stack: stack,
		theme: transcript.theme, glyphs: transcript.glyphs, clipboard: (&program.Runtime{}).Clipboard(),
		session: &sessionState{context: newSessionContextLease()},
	}
	if operations != nil {
		host.tasks = unattachedTasks(operations)
		host.reader = testReaderSurface(t, operations)
	}
	return host
}

// testPromptComposer is a composer whose notices, health reports and
// capability checks go nowhere.
func testPromptComposer(t *testing.T) *promptComposer {
	t.Helper()
	return &promptComposer{
		session: &sessionState{}, ctx: t.Context(),
		after:  func(time.Duration, func()) func() { return func() {} },
		closed: func() bool { return false }, notify: func(string) {},
		report: func(workbenchConcern, error) {}, validate: func(authoring.Message) error { return nil },
		appendNote: func(string, string) {}, cleared: func() {},
	}
}
