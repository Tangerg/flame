package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	coreDiff "github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/highlight"
)

func TestToolObserverIdentityExhaustionPreservesExistingSubscription(t *testing.T) {
	oldCalls, replacementCalls := 0, 0
	block := &toolBlock{}
	unsubscribeOld := block.Observe(func(readerDocument) { oldCalls++ })
	unsubscribeReplacement := block.Observe(func(readerDocument) { replacementCalls++ })
	unsubscribeReplacement()
	unsubscribeReplacement()
	block.observers.notify(readerDocument{})

	if oldCalls != 2 || replacementCalls != 1 {
		t.Fatalf("observer calls after replacement retirement = old %d replacement %d", oldCalls, replacementCalls)
	}
	unsubscribeOld()
}

func TestPluginPresenterPanicBecomesAnError(t *testing.T) {
	_, err := presentSafely(BlockPresenter{
		Kind: conversation.BlockAssistant,
		Present: func(BlockPresentation, conversation.Block) []headless.Block {
			panic("present boom")
		},
	}, BlockPresentation{}, conversation.Block{Kind: conversation.BlockAssistant})
	if err == nil || !strings.Contains(err.Error(), "present boom") {
		t.Fatalf("presenter panic error = %v", err)
	}
}

func TestCustomEventPresenterPanicBecomesAnError(t *testing.T) {
	_, err := presentCustomSafely(CustomEventPresenter{
		Name: "vendor.broken",
		Present: func(BlockPresentation, conversation.CustomEvent) []headless.Block {
			panic("custom boom")
		},
	}, BlockPresentation{}, conversation.CustomEvent{Name: "vendor.broken", PayloadJSON: []byte(`null`)})
	if err == nil || !strings.Contains(err.Error(), "custom boom") {
		t.Fatalf("custom presenter panic error = %v", err)
	}
}

func TestToolPresentersUseOrderedMatchingAndAGenericFallback(t *testing.T) {
	call := conversation.ToolCall{Kind: conversation.ToolUnknown, Name: "provider_tool", Summary: "work", Status: conversation.ToolRunning}
	presenters := []ToolPresenter{
		{
			ID:      "specific",
			Matches: func(got conversation.ToolCall) bool { return got.Name == "provider_tool" },
			Present: func(conversation.ToolCall) ToolPresentation { return ToolPresentation{Label: "specific view"} },
		},
		{ID: "fallback", Matches: func(conversation.ToolCall) bool { return true }, Present: presentUnknownTool},
	}
	presentation, err := selectToolPresentation(presenters, call)
	if err != nil {
		t.Fatal(err)
	}
	if presentation.Label != "specific view" {
		t.Fatalf("selected label = %q", presentation.Label)
	}

	fallback, err := selectToolPresentation(nil, conversation.ToolCall{Kind: conversation.ToolUnknown, Name: "other", Status: conversation.ToolRunning})
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Label != "other" {
		t.Fatalf("fallback label = %q", fallback.Label)
	}
}

func TestToolPresenterPanicsBecomePresentationErrors(t *testing.T) {
	presenter := ToolPresenter{
		ID:      "broken",
		Matches: func(conversation.ToolCall) bool { return true },
		Present: func(conversation.ToolCall) ToolPresentation { panic("projection boom") },
	}
	_, err := selectToolPresentation([]ToolPresenter{presenter}, conversation.ToolCall{})
	if err == nil || !strings.Contains(err.Error(), "projection boom") {
		t.Fatalf("tool presenter error = %v", err)
	}
}

func TestParseUnifiedDiffCarriesLineKindsAndNumbers(t *testing.T) {
	hunks := parseUnifiedDiff("--- a/a.go\n+++ b/a.go\n@@ -10,3 +10,3 @@\n keep\n-old\n+new\n")
	lines := requireSingleHunk(t, hunks, 10, 10, 3)
	if lines[0].Old != 10 || lines[0].New != 10 || lines[1].Old != 11 || lines[1].New != 0 || lines[2].Old != 0 || lines[2].New != 11 {
		t.Fatalf("numbered lines = %+v", lines)
	}
}

func requireSingleHunk(t *testing.T, hunks []coreDiff.Hunk, oldStart, newStart, lines int) []coreDiff.Line {
	t.Helper()
	if len(hunks) != 1 || hunks[0].Old != oldStart || hunks[0].New != newStart || len(hunks[0].Lines) != lines {
		t.Fatalf("hunks = %+v", hunks)
	}
	return hunks[0].Lines
}

func TestToolLabelUsesSemanticKindInsteadOfProviderName(t *testing.T) {
	call := conversation.ToolCall{Kind: conversation.ToolShell, Name: "opaque_provider_17", Command: "go test ./...", Summary: "ignored fallback"}
	if got := toolLabel(call); got != "$ go test ./..." || strings.Contains(got, call.Name) {
		t.Fatalf("label = %q", got)
	}
	call = conversation.ToolCall{Kind: conversation.ToolUnknown, Name: "custom", Summary: "do work"}
	if got := toolLabel(call); got != "custom · do work" {
		t.Fatalf("unknown label = %q", got)
	}
}

func TestToolDetailTruncationKeepsTheBeginningAndEnd(t *testing.T) {
	lines := make([]string, maxToolDetailLines+50)
	for i := range lines {
		lines[i] = "line " + string(rune(0x1000+i))
	}
	got := truncateToolDetail(strings.Join(lines, "\n"))
	if !strings.Contains(got, lines[0]) || !strings.Contains(got, lines[len(lines)-1]) || !strings.Contains(got, "70 lines omitted") {
		t.Fatalf("truncated detail did not preserve context: %q", got)
	}
}

func TestToolKindsBuildSpecializedOolongBlocks(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	diff := "--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
	tests := []struct {
		name string
		call conversation.ToolCall
		want string
	}{
		{name: "shell", call: conversation.ToolCall{Kind: conversation.ToolShell, Status: conversation.ToolOK, Output: "ok"}, want: "code"},
		{name: "read", call: conversation.ToolCall{Kind: conversation.ToolRead, Status: conversation.ToolOK, Path: "main.go", Output: "package main"}, want: "numbered-code"},
		{name: "edit", call: conversation.ToolCall{Kind: conversation.ToolEdit, Status: conversation.ToolOK, Path: "a.go", Diff: diff}, want: "diff"},
		{name: "search", call: conversation.ToolCall{Kind: conversation.ToolSearch, Status: conversation.ToolOK, Query: "needle", Output: "a.go:1"}, want: "paragraph"},
		{name: "web", call: conversation.ToolCall{Kind: conversation.ToolWeb, Status: conversation.ToolOK, URL: "https://example.com", Output: "https://example.com/result"}, want: "linked-paragraph"},
		{name: "task", call: conversation.ToolCall{Kind: conversation.ToolTask, Status: conversation.ToolOK, Summary: "delegate", Output: "done"}, want: "paragraph"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := newToolBlock(presentation, conversation.Block{ID: test.name, Kind: conversation.BlockTool, Tool: &test.call})
			if len(block.body) == 0 {
				t.Fatal("tool built no detail body")
			}
			requireToolBody(t, block.body[0], test.want)
		})
	}
}

func TestUnknownToolPresentsCompleteArgumentsAndResult(t *testing.T) {
	call := conversation.ToolCall{
		Kind: conversation.ToolUnknown, Name: "mcp__calendar__create", Status: conversation.ToolOK,
		ArgumentsJSON: []byte(`{"calendar":"work","guests":["a@example.com"]}`),
		ResultJSON:    []byte(`{"eventId":"evt_123","accepted":true}`),
	}
	presentation := presentUnknownTool(call)
	if len(presentation.Sections) != 2 || presentation.Sections[0].Title != "Arguments" ||
		presentation.Sections[1].Title != "Result" ||
		!strings.Contains(presentation.Sections[0].Text, "a@example.com") ||
		!strings.Contains(presentation.Sections[1].Text, "evt_123") {
		t.Fatalf("unknown tool presentation = %+v", presentation)
	}
}

func TestKnownToolAlsoPresentsCompleteArgumentsAndResult(t *testing.T) {
	call := conversation.ToolCall{
		Kind: conversation.ToolShell, Command: "go test ./...", Status: conversation.ToolOK,
		ArgumentsJSON: []byte(`{"command":"go test ./...","timeoutMs":30000}`),
		ResultJSON:    []byte(`{"exitCode":0,"truncated":false}`),
	}
	presentation := presentShellTool(call)
	if len(presentation.Sections) != 2 || presentation.Sections[0].Title != "Arguments" ||
		presentation.Sections[1].Title != "Result" ||
		!strings.Contains(presentation.Sections[0].Text, "timeoutMs") ||
		!strings.Contains(presentation.Sections[1].Text, "truncated") {
		t.Fatalf("known tool presentation = %+v", presentation)
	}
}

func TestToolDetailsPresentSafetyAndLifecycleMetadata(t *testing.T) {
	started := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	presentation := presentShellTool(conversation.ToolCall{
		Kind: conversation.ToolShell, Command: "go test ./...", Status: conversation.ToolOK,
		Safety: protocol.SafetyClassExec, StartedAt: started, FinishedAt: started.Add(2 * time.Second),
	})
	if len(presentation.Sections) == 0 || presentation.Sections[0].Title != "Execution" ||
		!strings.Contains(presentation.Sections[0].Text, "safety   exec") ||
		!strings.Contains(presentation.Sections[0].Text, "started  2026-08-12T09:00:00Z") ||
		!strings.Contains(presentation.Sections[0].Text, "finished 2026-08-12T09:00:02Z") {
		t.Fatalf("tool lifecycle presentation = %+v", presentation)
	}
}

func TestToolDetailsPreserveStructuredProblems(t *testing.T) {
	presentation := presentUnknownTool(conversation.ToolCall{
		Kind: conversation.ToolUnknown, Name: "provider_tool", Status: conversation.ToolError,
		Problem: &protocol.ProblemData{
			Type:   protocol.ProblemToolFailed,
			DocURL: "https://docs.example/errors/rate-limit",
		},
	})
	if len(presentation.Sections) != 1 || presentation.Sections[0].Title != "Problem" ||
		!strings.Contains(presentation.Sections[0].Text, "tool_failed") ||
		!strings.Contains(presentation.Sections[0].Text, "docs.example") {
		t.Fatalf("tool problem presentation = %+v", presentation)
	}
}

func TestToolBlockOwnsCompleteToolValues(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	call := conversation.ToolCall{
		Kind: conversation.ToolUnknown, Name: "provider_tool", Status: conversation.ToolOK,
		ArgumentsJSON: []byte(`{"scope":"source"}`), ResultJSON: []byte(`{"status":"source"}`),
	}
	block := newToolBlock(presentation, conversation.Block{ID: "tool", Kind: conversation.BlockTool, Tool: &call})
	copy(call.ArgumentsJSON, `{"scope":"mutant"}`)
	copy(call.ResultJSON, `{"status":"mutant"}`)

	if strings.Contains(string(block.call.ArgumentsJSON), "mutant") || strings.Contains(string(block.call.ResultJSON), "mutant") {
		t.Fatalf("tool block retained caller-owned JSON: %+v", block.call)
	}
}

func TestUpdatingARunningToolPreservesItsDetailChoice(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	running := conversation.ToolCall{Kind: conversation.ToolShell, Command: "go test ./...", Status: conversation.ToolRunning}
	block := newToolBlock(presentation, conversation.Block{ID: "tool", Kind: conversation.BlockTool, Tool: &running})
	block.ToggleExpanded()

	completed := running
	completed.Status = conversation.ToolOK
	completed.Output = "ok"
	block.Update(conversation.Block{ID: "tool", Kind: conversation.BlockTool, Tool: &completed})
	if !block.Expanded() {
		t.Fatal("tool completion discarded the reader's expanded state")
	}
}

func TestToolBlockStreamsOutputWithoutLosingItsDetailChoice(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	running := conversation.ToolCall{Kind: conversation.ToolShell, Command: "go test ./...", Status: conversation.ToolRunning}
	block := newToolBlock(presentation, conversation.Block{ID: "tool", Kind: conversation.BlockTool, Tool: &running})
	if !block.Expandable() {
		t.Fatal("running tool was not expandable before its first output")
	}
	block.SetExpanded(true)
	block.AppendOutput("first\n")
	block.AppendOutput("second\n")
	if !block.Expanded() {
		t.Fatal("streaming output discarded the expanded state")
	}
	if got := block.call.Output; got != "first\nsecond\n" {
		t.Fatalf("streamed output = %q", got)
	}
	drawn := drawToolBlock(block, 48)
	if !strings.Contains(drawn, "first") || !strings.Contains(drawn, "second") {
		t.Fatalf("streamed output was not rendered:\n%s", drawn)
	}
}

func TestCompletedToolWithoutDetailsCannotExpand(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	completed := conversation.ToolCall{Kind: conversation.ToolShell, Command: "true", Status: conversation.ToolOK}
	block := newToolBlock(presentation, conversation.Block{ID: "tool", Kind: conversation.BlockTool, Tool: &completed})
	if block.Expandable() || block.Expanded() {
		t.Fatal("detail-free completed tool was expandable")
	}
	block.SetExpanded(true)
	if block.ToggleExpanded() || block.Expanded() {
		t.Fatal("detail-free completed tool accepted an expansion request")
	}
	if got := block.HeightForWidth(48); got != 2 {
		t.Fatalf("detail-free tool height = %d, want header plus gap", got)
	}
	toggle, _, _, _ := block.header()
	if toggle != presentation.Glyphs.Bullet {
		t.Fatalf("detail-free tool toggle = %q, want bullet %q", toggle, presentation.Glyphs.Bullet)
	}
}

func TestToolBlockDrawsALocaleSafeStatusRailThroughExpandedDetails(t *testing.T) {
	theme, glyphs := kit.Dark(), kit.ASCII()
	call := conversation.ToolCall{
		Kind: conversation.ToolShell, Command: "go test ./...", Status: conversation.ToolOK, Output: "all packages passed",
	}
	block := newToolBlock(BlockPresentation{Theme: theme, Glyphs: glyphs, Syntax: highlight.New("github-dark")}, conversation.Block{
		ID: "test", Kind: conversation.BlockTool, Tool: &call,
	})
	block.SetExpanded(true)
	width, height := 48, block.HeightForWidth(48)
	surface := grid.NewSurface(width, height)
	block.Draw(surface.View())

	drawn := strings.Join(surface.Rows(), "\n")
	for _, want := range []string{"- $ go test ./...", "x done", "all packages passed"} {
		if !strings.Contains(drawn, want) {
			t.Errorf("tool block does not contain %q:\n%s", want, drawn)
		}
	}
	for row := range height - 1 {
		cell, ok := surface.CellAt(0, row)
		if !ok || cell.Content() != glyphs.Vertical || cell.Style != theme.Success {
			t.Fatalf("status rail row %d = %+v, want %q with success style", row, cell, glyphs.Vertical)
		}
	}
	if strings.Contains(drawn, "✓") || strings.Contains(drawn, "…") {
		t.Fatalf("ASCII tool block contains a Unicode-only status glyph:\n%s", drawn)
	}

	rows := block.Rows(width)
	if len(rows) != height || strings.HasPrefix(rows[0].Text, glyphs.Vertical) {
		t.Fatalf("copied rows include visual rail or have wrong height: %+v", rows)
	}
}

func TestToolStatusVocabularyDoesNotCollideWithRunOutcomes(t *testing.T) {
	presentation := BlockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode()}
	for _, test := range []struct {
		status conversation.ToolStatus
		want   string
	}{
		{status: conversation.ToolOK, want: "done"},
		{status: conversation.ToolError, want: "error"},
		{status: conversation.ToolCanceled, want: "canceled"},
		{status: conversation.ToolRunning, want: "running"},
	} {
		call := conversation.ToolCall{Kind: conversation.ToolTask, Status: test.status}
		block := newToolBlock(presentation, conversation.Block{Kind: conversation.BlockTool, Tool: &call})
		_, _, status, _ := block.header()
		if !strings.Contains(status, test.want) || strings.Contains(status, "complete") || strings.Contains(status, "failed") {
			t.Errorf("tool status %q = %q", test.status, status)
		}
	}
}

func requireToolBody(t *testing.T, body headless.Block, want string) {
	t.Helper()
	switch want {
	case "code":
		requireCodeBody(t, body, false)
	case "numbered-code":
		requireCodeBody(t, body, true)
	case "diff":
		requireBodyType[*kit.Diff](t, body, "diff")
	case "paragraph":
		requireBodyType[*kit.Paragraph](t, body, "paragraph")
	case "linked-paragraph":
		requireLinkedParagraph(t, body)
	}
}

func requireCodeBody(t *testing.T, body headless.Block, numbered bool) {
	t.Helper()
	code, ok := body.(*kit.Code)
	if !ok {
		t.Fatalf("body = %T, want code", body)
	}
	if numbered && code.Gutter == nil {
		t.Fatalf("body = %#v, want numbered code", body)
	}
}

func requireBodyType[T any](t *testing.T, body headless.Block, name string) {
	t.Helper()
	if _, ok := body.(T); !ok {
		t.Fatalf("body = %T, want %s", body, name)
	}
}

func requireLinkedParagraph(t *testing.T, body headless.Block) {
	t.Helper()
	paragraph, ok := body.(*kit.Paragraph)
	if !ok {
		t.Fatalf("body = %T, want linked paragraph", body)
	}
	destination, ok := paragraph.LinkAt(0, 0, 80)
	if !ok || destination.Target != "https://example.com/result" {
		t.Fatalf("link = %#v, %v, want detected web destination", destination, ok)
	}
}

func drawToolBlock(block *toolBlock, width int) string {
	surface := grid.NewSurface(width, block.HeightForWidth(width))
	block.Draw(surface.View())
	return strings.Join(surface.Rows(), "\n")
}

// TestSectionLanguageDefaultsToPlainText pins the answer sixteen code sections
// stopped repeating. Rendering does not observe the language, so nothing else
// would notice this default changing.
func TestSectionLanguageDefaultsToPlainText(t *testing.T) {
	t.Parallel()
	if got := sectionLanguage(ToolSection{}); got != "text" {
		t.Fatalf("unset section language = %q, want plain text", got)
	}
	if got := sectionLanguage(ToolSection{Language: "go"}); got != "go" {
		t.Fatalf("declared section language = %q, want it kept", got)
	}
}
