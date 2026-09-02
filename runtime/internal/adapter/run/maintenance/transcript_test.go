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
	if measured := transcriptBytes(messages); measured != len(rendered) {
		t.Fatalf("transcriptBytes = %d, want rendered size %d", measured, len(rendered))
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
