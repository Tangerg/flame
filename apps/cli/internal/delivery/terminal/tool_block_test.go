package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestTranscriptPresenterPanicBecomesAnError(t *testing.T) {
	_, err := presentSafely(func(blockPresentation, conversation.Block) []base.Block { panic("present boom") }, blockPresentation{}, conversation.Block{Kind: conversation.BlockAssistant})
	if err == nil || !strings.Contains(err.Error(), "present boom") {
		t.Fatalf("presenter panic error = %v", err)
	}
}

func TestToolPresentersUseOrderedMatchingAndAGenericFallback(t *testing.T) {
	call := conversation.ToolCall{Name: "provider_tool", Summary: "work", Status: conversation.ToolRunning}
	presenters := []toolPresenter{
		{
			ID:      "specific",
			Matches: func(got conversation.ToolCall) bool { return got.Name == "provider_tool" },
			Present: func(conversation.ToolCall) toolPresentation { return toolPresentation{Label: "specific view"} },
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

	fallback, err := selectToolPresentation(nil, conversation.ToolCall{Name: "other", Status: conversation.ToolRunning})
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Label != "other" {
		t.Fatalf("fallback label = %q", fallback.Label)
	}
}

func TestToolPresenterPanicsBecomePresentationErrors(t *testing.T) {
	presenter := toolPresenter{
		ID:      "broken",
		Matches: func(conversation.ToolCall) bool { return true },
		Present: func(conversation.ToolCall) toolPresentation { panic("projection boom") },
	}
	_, err := selectToolPresentation([]toolPresenter{presenter}, conversation.ToolCall{})
	if err == nil || !strings.Contains(err.Error(), "projection boom") {
		t.Fatalf("tool presenter error = %v", err)
	}
}

func TestRawToolPatchPreservesFileHeadersAndMarkers(t *testing.T) {
	patch := "diff --git a/one.txt b/one.txt\n--- a/one.txt\n+++ b/one.txt\n@@ -1 +1 @@\n-old one\n+new one\ndiff --git a/two.txt b/two.txt\n--- a/two.txt\n+++ b/two.txt\n@@ -1 +1 @@\n-old two\n+new two\n\\ No newline at end of file"
	presentation := presentEditTool(conversation.ToolCall{Kind: builtin.CategoryEdit, Diff: patch})
	blocks := renderToolSections(blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}, presentation.Sections, false)
	var copied []string
	for _, block := range blocks {
		projector, ok := block.(text.Projector)
		if !ok {
			t.Fatalf("raw patch cannot be copied: %T", block)
		}
		copied = append(copied, copyableRowsText(projector.Rows(120)))
	}
	if got := strings.Join(copied, "\n"); got != patch {
		t.Fatalf("raw patch = %q, want %q", got, patch)
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

func TestUnknownToolPresentsCompleteArgumentsAndResult(t *testing.T) {
	call := conversation.ToolCall{
		Name: "mcp__calendar__create", Status: conversation.ToolOK,
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
		Kind: builtin.CategoryShell, Command: "go test ./...", Status: conversation.ToolOK,
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
		Kind: builtin.CategoryShell, Command: "go test ./...", Status: conversation.ToolOK,
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
		Name: "provider_tool", Status: conversation.ToolError,
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

func TestUpdatingARunningToolPreservesItsDetailChoice(t *testing.T) {
	presentation := blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	running := conversation.ToolCall{Kind: builtin.CategoryShell, Command: "go test ./...", Status: conversation.ToolRunning}
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
	presentation := blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	running := conversation.ToolCall{Kind: builtin.CategoryShell, Command: "go test ./...", Status: conversation.ToolRunning}
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
	presentation := blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode(), Syntax: highlight.New("github-dark")}
	completed := conversation.ToolCall{Kind: builtin.CategoryShell, Command: "true", Status: conversation.ToolOK}
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

func drawToolBlock(block *toolBlock, width int) string {
	surface := grid.NewSurface(width, block.HeightForWidth(width))
	block.Draw(surface.View())
	return strings.Join(surface.Rows(), "\n")
}
