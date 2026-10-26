package capture

import (
	"errors"
	"strings"
	"testing"
)

func TestCapturedPagesHaveImmutableContentIdentityAndUnicodeOffsets(t *testing.T) {
	content := Content{URL: "https://example.test/article", Format: Markdown, Text: strings.Repeat("知识", 5000)}
	document, err := NewDocument(content)
	if err != nil {
		t.Fatal(err)
	}
	first, err := document.Page(0, MaxPageRunes)
	if err != nil || first.TotalRunes != 10000 || first.NextOffset == nil || *first.NextOffset != MaxPageRunes {
		t.Fatalf("first page: %+v %v", first, err)
	}
	last, err := document.Page(*first.NextOffset, MaxPageRunes)
	if err != nil || last.NextOffset != nil || first.Content+last.Content != content.Text {
		t.Fatalf("Unicode continuation: %+v %v", last, err)
	}
	for _, offset := range []int64{-1, 10001, 1 << 62} {
		if _, err := document.Page(offset, 1); !errors.Is(err, ErrRange) {
			t.Fatalf("invalid offset: %v", err)
		}
	}
	tampered := document.Snapshot()
	tampered.Text = "forged"
	if _, err := Restore(tampered); !errors.Is(err, ErrDocument) {
		t.Fatalf("snapshot identity accepted changed content: %v", err)
	}
	content.Text = "changed source"
	changed, err := NewDocument(content)
	if err != nil || changed.Snapshot().ID == document.Snapshot().ID {
		t.Fatal("new content reused an old snapshot identity")
	}
	for _, content := range []Content{{URL: "file:///private/data", Format: Text}, {URL: "https://user:secret@example.test", Format: Text}, {URL: "https://example.test", Format: Text, Text: strings.Repeat("x", MaxDocumentBytes+1)}} {
		if _, err := NewDocument(content); !errors.Is(err, ErrDocument) {
			t.Fatalf("invalid capture admitted: %v", err)
		}
	}
}
