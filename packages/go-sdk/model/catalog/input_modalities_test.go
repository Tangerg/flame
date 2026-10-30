package catalog

import (
	"errors"
	"strings"
	"testing"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

func TestAdmissionUsesExactCatalogModel(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	messages := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("inspect"), chat.NewMediaPart(image)),
	}
	textOnly, err := modelchoice.New("alibaba", "qwen-mt-plus")
	if err != nil {
		t.Fatal(err)
	}
	if admitErr := referenceAdmission(t, textOnly).AdmitInput(messages); !errors.Is(admitErr, ErrUnsupportedInputModality) {
		t.Fatalf("text-only model error = %v, want ErrUnsupportedInputModality", admitErr)
	}

	imageCapable, err := modelchoice.New("openai", "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	if admitErr := referenceAdmission(t, imageCapable).AdmitInput(messages); admitErr != nil {
		t.Fatalf("image-capable model error = %v", admitErr)
	}
}

func TestAdmissionRejectsMissingModelSelection(t *testing.T) {
	err := (Admission{}).AdmitInput([]chat.Message{
		chat.NewUserMessage(chat.NewTextPart("inspect")),
	})
	if err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("missing model selection error = %v", err)
	}
}

func TestAdmissionIncludesToolResultMedia(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	messages := []chat.Message{chat.NewToolMessage(chat.ToolResult{
		ID: "image-result", Name: "inspect",
		Output: chat.ToolOutput{Content: []chat.ToolContent{
			{Kind: chat.PartText, Text: "Inspect this image"},
			{Kind: chat.PartMedia, Media: image},
		}},
	})}
	for _, test := range []struct {
		provider, model string
		accepts         bool
	}{
		{provider: "alibaba", model: "qwen-mt-plus"},
		{provider: "openai", model: "gpt-5.6-sol", accepts: true},
	} {
		selection, err := modelchoice.New(test.provider, test.model)
		if err != nil {
			t.Fatal(err)
		}
		err = referenceAdmission(t, selection).AdmitInput(messages)
		if test.accepts && err != nil {
			t.Fatalf("image-capable model rejected Tool content: %v", err)
		}
		if !test.accepts && !errors.Is(err, ErrUnsupportedInputModality) {
			t.Fatalf("text-only model admitted Tool media: %v", err)
		}
	}
}

func TestAdmissionAllowsPrivateCatalogMiss(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	selection, err := modelchoice.New("openai-compatible", "private-vision-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := referenceAdmission(t, selection).AdmitInput([]chat.Message{
		chat.NewUserMessage(chat.NewMediaPart(image)),
	}); err != nil {
		t.Fatalf("private model error = %v", err)
	}
}

func TestCatalogInputModalityClassifiesOnlyModelContent(t *testing.T) {
	text, relevant, err := catalogInputModality(chat.NewTextPart("hello"))
	if err != nil || !relevant || text != "text" {
		t.Fatalf("text modality = (%q, %t, %v), want (text, true, nil)", text, relevant, err)
	}
	_, relevant, err = catalogInputModality(chat.NewReasoningPart("thinking", nil))
	if err != nil || relevant {
		t.Fatalf("reasoning modality = (_, %t, %v), want (_, false, nil)", relevant, err)
	}
}

func TestCatalogInputModalityRejectsUnsupportedMediaType(t *testing.T) {
	archive, err := media.NewBytes("application/zip", []byte("archive"))
	if err != nil {
		t.Fatal(err)
	}
	_, relevant, err := catalogInputModality(chat.NewMediaPart(archive))
	if !relevant || err == nil {
		t.Fatalf("archive modality = (_, %t, %v), want relevant error", relevant, err)
	}
}
