package runtimebinding

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func TestProjectInputReadsTypedAttachmentsAtDispatch(t *testing.T) {
	directory := t.TempDir()
	textPath := filepath.Join(directory, "notes.txt")
	imagePath := filepath.Join(directory, "pixel.png")
	if err := os.WriteFile(textPath, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	image := []byte{0x89, 'P', 'N', 'G'}
	if err := os.WriteFile(imagePath, image, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := &Connection{
		loadAttachment: loadAttachmentFile,
		profile: Profile{Features: map[string]Feature{
			protocol.FeatureMultimodal: {Enabled: true},
		}},
	}
	blocks, err := runtime.projectInput(t.Context(), agent.Message{
		Text: "prompt",
		Attachments: []agent.Attachment{
			{ID: "text", Kind: protocol.ContentBlockText, Name: "notes.txt", Path: textPath, MimeType: "text/plain", Size: 5},
			{ID: "image", Kind: protocol.ContentBlockImage, Name: "pixel.png", Path: imagePath, MimeType: "image/png", Size: int64(len(image))},
		},
	})
	if err != nil {
		t.Fatalf("projectInput: %v", err)
	}
	if len(blocks) != 3 || blocks[0].Type != protocol.ContentBlockText || blocks[0].Text != "prompt" ||
		blocks[1].Type != protocol.ContentBlockText || blocks[2].Type != protocol.ContentBlockImage ||
		blocks[2].Data != base64.StdEncoding.EncodeToString(image) {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestProjectInputPreservesMeaningfulTextAndOmitsBlankText(t *testing.T) {
	runtime := &Connection{loadAttachment: func(context.Context, string, int64) ([]byte, error) {
		return []byte("notes"), nil
	}}

	const authored = "  indented\ntrailing  \n"
	blocks, err := runtime.projectInput(t.Context(), agent.Message{Text: authored})
	if err != nil || len(blocks) != 1 || blocks[0].Text != authored {
		t.Fatalf("meaningful projection = (%+v, %v)", blocks, err)
	}

	blocks, err = runtime.projectInput(t.Context(), agent.Message{
		Text: " \n\t",
		Attachments: []agent.Attachment{{
			ID: "text", Kind: protocol.ContentBlockText, Name: "notes.txt", Path: "/notes.txt",
			MimeType: "text/plain", Size: 5,
		}},
	})
	if err != nil {
		t.Fatalf("attachment-only projection: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Type != protocol.ContentBlockText || !strings.Contains(blocks[0].Text, "notes") {
		t.Fatalf("attachment-only blocks = %+v", blocks)
	}
}

func TestProjectInputRejectsImagesBeforeReadingWithoutMultimodalCapability(t *testing.T) {
	t.Parallel()
	reads := 0
	runtime := &Connection{loadAttachment: func(context.Context, string, int64) ([]byte, error) {
		reads++
		return []byte("image"), nil
	}}
	blocks, err := runtime.projectInput(t.Context(), agent.Message{Attachments: []agent.Attachment{{
		ID: "image", Kind: protocol.ContentBlockImage, Name: "image.png", Path: "/image.png",
		MimeType: "image/png", Size: 5,
	}}})
	if err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("projectInput error = %v, want ErrIncompatibleRuntime", err)
	}
	if blocks != nil || reads != 0 {
		t.Fatalf("projectInput = (%+v, %v), want no blocks or attachment reads", blocks, reads)
	}
}

func TestProjectInputRejectsInvalidTextBytesAtDispatch(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "invalid UTF-8", data: []byte{'o', 'k', 0xff}},
		{name: "NUL", data: []byte{'o', 'k', 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &Connection{loadAttachment: func(context.Context, string, int64) ([]byte, error) {
				return test.data, nil
			}}
			blocks, err := runtime.projectInput(t.Context(), agent.Message{Attachments: []agent.Attachment{{
				ID: "text", Kind: protocol.ContentBlockText, Name: "notes.txt", Path: "/notes.txt",
				MimeType: "text/plain", Size: int64(len(test.data)),
			}}})
			if err == nil || !strings.Contains(err.Error(), "not valid text") {
				t.Fatalf("projectInput error = %v, want invalid text", err)
			}
			if blocks != nil {
				t.Fatalf("projectInput blocks = %+v, want nil", blocks)
			}
		})
	}
}

func TestLoadAttachmentFileEnforcesTheReadLimit(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError bool
	}{
		{name: "at limit", content: "12345678"},
		{name: "over limit", content: "123456789", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "attachment.txt")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}

			data, err := loadAttachmentFile(t.Context(), path, 8)
			if !test.wantError {
				if err != nil || string(data) != test.content {
					t.Fatalf("loadAttachmentFile = (%q, %v), want (%q, nil)", data, err, test.content)
				}
				return
			}
			var sizeError attachmentTooLargeError
			if !errors.As(err, &sizeError) || sizeError.maximumBytes != 8 {
				t.Fatalf("loadAttachmentFile error = %v, want 8-byte attachmentTooLargeError", err)
			}
			if data != nil {
				t.Fatalf("loadAttachmentFile data = %q, want nil", data)
			}
		})
	}
}

func TestLoadAttachmentFileHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	data, err := loadAttachmentFile(ctx, path, 8)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("loadAttachmentFile error = %v, want context.Canceled", err)
	}
	if data != nil {
		t.Fatalf("loadAttachmentFile data = %q, want nil", data)
	}
}

func TestLoadAttachmentFileRejectsNonRegularSources(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.txt")
	if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "attachment.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{directory, link} {
		data, err := loadAttachmentFile(t.Context(), path, 8)
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("loadAttachmentFile(%q) error = %v, want non-regular source", path, err)
		}
		if data != nil {
			t.Fatalf("loadAttachmentFile(%q) data = %q, want nil", path, data)
		}
	}
}

func TestProjectContentCreatesHonestDurableImageReference(t *testing.T) {
	text, attachments, err := projectContent("item_1", []protocol.ContentBlock{
		{Type: protocol.ContentBlockText, Text: "hello"},
		{Type: protocol.ContentBlockImage, Mime: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("image"))},
	})
	if err != nil {
		t.Fatalf("projectContent: %v", err)
	}
	if text != "hello" || len(attachments) != 1 || attachments[0].Path != "" || attachments[0].MimeType != "image/png" {
		t.Fatalf("content = (%q, %+v)", text, attachments)
	}
	if err := attachments[0].Validate(); err != nil {
		t.Fatalf("durable image: %v", err)
	}
}
