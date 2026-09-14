package maintenance

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/scope/core/chat"
)

func TestTranscriptPreservesRefusalText(t *testing.T) {
	messages := []chat.Message{
		chat.NewAssistantMessage(chat.NewRefusalPart("I cannot help with that request.")),
	}

	rendered := renderTranscript(messages)
	if !strings.Contains(rendered, "I cannot help with that request.") {
		t.Fatalf("renderTranscript = %q, want refusal text", rendered)
	}
}

func TestCapText(t *testing.T) {
	// Disabled (max<=0) or already-small: returned unchanged.
	if got := capText("hello", 0); got != "hello" {
		t.Fatalf("cap<=0 should pass through, got %q", got)
	}
	if got := capText("hello", 100); got != "hello" {
		t.Fatalf("len<=max should pass through, got %q", got)
	}

	// Oversized: capped shorter than input, marked, head + tail preserved.
	big := strings.Repeat("x", 10_000)
	got := capText(big, 400)
	if len(got) > 400 {
		t.Fatalf("capText exceeded exact byte limit: %d > 400", len(got))
	}
	if len(got) >= len(big) {
		t.Fatalf("capText did not shrink: %d >= %d", len(got), len(big))
	}
	if !strings.Contains(got, "elided for auxiliary model input") {
		t.Fatal("missing elision marker")
	}
	if !strings.HasPrefix(got, "xxx") || !strings.HasSuffix(got, "xxx") {
		t.Fatalf("head/tail not preserved: %q", got)
	}

	// Rune-safe: a body of multibyte runes stays valid UTF-8 after the cut,
	// regardless of where the raw byte offsets land.
	runes := strings.Repeat("世界", 5_000) // 3 bytes per rune
	if capped := capText(runes, 401); !utf8.ValidString(capped) {
		t.Fatal("capText split a multibyte rune (invalid UTF-8)")
	}
}

func TestTranscriptPreservesToolCallAndFailureIdentity(t *testing.T) {
	messages := []chat.Message{
		chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: "patch-1", Name: "apply_patch", Arguments: `{"patch":"diff --git a/client.go b/client.go"}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "test-1", Name: "shell", Arguments: `{"command":"go test ./..."}`}),
		),
		chat.NewToolMessage(
			chat.ToolResult{ID: "patch-1", Name: "apply_patch", IsError: true, Output: chat.NewTextToolOutput("hunk did not match")},
			chat.ToolResult{ID: "test-1", Name: "shell", Output: chat.NewTextToolOutput("ok")},
		),
	}
	rendered := renderTranscript(messages)
	for _, want := range []string{"apply_patch", "patch-1", "diff --git a/client.go b/client.go", "shell", "test-1", "go test ./...", "error=true", "error=false", "hunk did not match"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("transcript missing %q: %s", want, rendered)
		}
	}
}

func TestTranscriptMarksMediaInsteadOfSilentlyDroppingIt(t *testing.T) {
	image := mustBudgetImage(t, []byte("private inline payload"))
	messages := []chat.Message{
		chat.NewUserMessage(chat.NewMediaPart(image)),
		chat.NewAssistantMessage(chat.NewTextPart("Compare the attachment."), chat.NewMediaPart(image)),
		budgetToolImage(image),
	}
	rendered := renderTranscript(messages)
	if strings.Count(rendered, "[media content omitted]") != 3 {
		t.Fatalf("media presence was lost: %q", rendered)
	}
	if !strings.Contains(rendered, "[user] [media content omitted]") || !strings.Contains(rendered, "Compare the attachment.") {
		t.Fatalf("media framing or adjacent text was lost: %q", rendered)
	}
	if strings.Contains(rendered, "private inline payload") {
		t.Fatal("binary media was rendered as transcript text")
	}
}
