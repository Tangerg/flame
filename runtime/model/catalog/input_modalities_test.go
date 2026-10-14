package modelcatalog

import (
	"errors"
	"strings"
	"testing"

	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

func TestSelectionsAdmitInputUsesExactCatalogModel(t *testing.T) {
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
	if admitErr := (Selections{}).AdmitInput(textOnly, messages); !errors.Is(admitErr, ErrUnsupportedInputModality) {
		t.Fatalf("text-only model error = %v, want ErrUnsupportedInputModality", admitErr)
	}

	imageCapable, err := modelchoice.New("openai", "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	if admitErr := (Selections{}).AdmitInput(imageCapable, messages); admitErr != nil {
		t.Fatalf("image-capable model error = %v", admitErr)
	}
}

func TestSelectionsAdmitInputRejectsMissingModelSelection(t *testing.T) {
	err := (Selections{}).AdmitInput(modelchoice.Selection{}, []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("inspect")),
	})
	if err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("missing model selection error = %v", err)
	}
}

func TestSelectionsAdmitInputAllowsPrivateCatalogMiss(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	selection, err := modelchoice.New("openai-compatible", "private-vision-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := (Selections{}).AdmitInput(selection, []chat.Message{
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
