package capture

import (
	"errors"
	"fmt"
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

func TestCredentialAbsenceIsDistinctFromInvalidConfigurationAndFormattingRedacts(t *testing.T) {
	unset, err := NewAPIKey(nil)
	if err != nil || unset.Configured() {
		t.Fatal("unset credential became configured")
	}
	for _, raw := range []string{"", " ", " key ", "key\n", string([]byte{0xff})} {
		if _, err := NewAPIKey(&raw); err == nil {
			t.Fatal("invalid configured credential was accepted")
		}
	}
	secret := "private-web-fixture-key"
	key, err := NewAPIKey(&secret)
	if err != nil || !key.Configured() || key.Value() != secret {
		t.Fatal("configured credential changed")
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		if strings.Contains(fmt.Sprintf(format, key), secret) {
			t.Fatalf("format %s disclosed a credential", format)
		}
	}
}
