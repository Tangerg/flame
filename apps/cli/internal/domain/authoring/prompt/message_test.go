package prompt

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestMessageSemanticTextDoesNotNormalizeAuthoredContent(t *testing.T) {
	for _, text := range []string{"", " \n\t"} {
		message := Message{Text: text}
		if message.HasText() || !message.IsEmpty() {
			t.Fatalf("blank message %q = hasText:%t empty:%t", text, message.HasText(), message.IsEmpty())
		}
	}

	const authored = "  indented\ntrailing  \n"
	message := Message{Text: authored}
	if !message.HasText() || message.IsEmpty() || message.Text != authored {
		t.Fatalf("authored message = %+v", message)
	}
	message.Attachments = []Attachment{{}}
	message.Text = " \n\t"
	if message.IsEmpty() {
		t.Fatal("attachment-only message was empty")
	}
}

func TestMessageRejectsOversizedText(t *testing.T) {
	message := Message{Text: strings.Repeat("x", MaxMessageTextBytes)}
	if err := message.Validate(); err != nil {
		t.Fatalf("maximum-sized message error = %v", err)
	}
	message.Text += "x"
	if err := message.Validate(); err == nil || !strings.Contains(err.Error(), "message text") {
		t.Fatalf("oversized message error = %v", err)
	}
}

func TestMessageRejectsDuplicateAttachments(t *testing.T) {
	attachment := Attachment{ID: "a", Kind: protocol.ContentBlockText, Name: "a.txt", Path: "/tmp/a.txt"}
	message := Message{Attachments: []Attachment{attachment, attachment}}
	if err := message.Validate(); err == nil {
		t.Fatal("duplicate attachment was accepted")
	}
}

func TestMessageRejectsOversizedAttachment(t *testing.T) {
	attachment := Attachment{
		ID: "a", Kind: protocol.ContentBlockText, Name: "large.txt", Path: "/tmp/large.txt",
		Size: MaxAttachmentBytes + 1,
	}
	if err := (Message{Attachments: []Attachment{attachment}}).Validate(); err == nil || !strings.Contains(err.Error(), "size exceeds") {
		t.Fatalf("oversized attachment error = %v", err)
	}
}

func TestDurableAttachmentMayLackLocalPathButDraftMayNot(t *testing.T) {
	durable := Attachment{ID: "item_1:image:0", Kind: protocol.ContentBlockImage, Name: "image.png", MimeType: "image/png", Size: 8}
	if err := durable.Validate(); err != nil {
		t.Fatalf("durable attachment: %v", err)
	}
	if err := (Message{Attachments: []Attachment{durable}}).Validate(); err == nil || !strings.Contains(err.Error(), "local path") {
		t.Fatalf("draft attachment error = %v", err)
	}
	invalidMIME := durable
	invalidMIME.MimeType = "text/plain"
	if err := invalidMIME.Validate(); err == nil || !strings.Contains(err.Error(), "image MIME") {
		t.Fatalf("image attachment MIME error = %v", err)
	}
}
