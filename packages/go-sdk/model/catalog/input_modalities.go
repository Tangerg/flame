package catalog

import (
	"errors"
	"fmt"
	"mime"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/models/catalog"
)

var ErrUnsupportedInputModality = errors.New("model: unsupported input modality")

// AdmitInput rejects media that a known catalog model does not
// accept. A catalog miss remains admissible because compatible endpoints may
// expose private models whose capabilities are not available locally.
func (a Admission) AdmitInput(messages []chat.Message) error {
	selection := a.selection
	if err := selection.ValidateExact(); err != nil {
		return fmt.Errorf("model: input-modality selection: %w", err)
	}
	if a.metadata == nil {
		return nil
	}
	entry := a.metadata
	for messageIndex := range messages {
		for partIndex := range messages[messageIndex].Parts {
			part := messages[messageIndex].Parts[partIndex]
			if err := admitCatalogPart(entry.Modalities, part); err != nil {
				return fmt.Errorf(
					"%w: model %q/%q message %d part %d: %w",
					ErrUnsupportedInputModality,
					selection.Provider(),
					selection.Model(),
					messageIndex,
					partIndex,
					err,
				)
			}
		}
	}
	return nil
}

func admitCatalogPart(allowed catalog.Modalities, part chat.Part) error {
	if part.Kind == chat.PartToolResult {
		if part.ToolResult == nil {
			return errors.New("tool result content is absent")
		}
		for index, content := range part.ToolResult.Output.Content {
			if err := admitCatalogPart(allowed, chat.Part{Kind: content.Kind, Media: content.Media}); err != nil {
				return fmt.Errorf("tool result content %d: %w", index, err)
			}
		}
		return nil
	}
	modality, relevant, err := catalogInputModality(part)
	if err != nil {
		return err
	}
	if relevant && !allowed.AcceptsInput(modality) {
		return fmt.Errorf("model does not accept %s input", modality)
	}
	return nil
}

func catalogInputModality(part chat.Part) (catalog.Modality, bool, error) {
	switch part.Kind {
	case chat.PartText:
		return catalog.ModalityText, true, nil
	case chat.PartMedia:
		modality, err := catalogMediaInputModality(part.Media)
		return modality, true, err
	default:
		return "", false, nil
	}
}

func catalogMediaInputModality(value *media.Media) (catalog.Modality, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	mediaType, _, _ := mime.ParseMediaType(value.MIME)
	major, _, _ := strings.Cut(mediaType, "/")
	switch major {
	case "image":
		return catalog.ModalityImage, nil
	case "audio":
		return catalog.ModalityAudio, nil
	case "video":
		return catalog.ModalityVideo, nil
	case "application":
		if mediaType == "application/pdf" {
			return catalog.ModalityPDF, nil
		}
	}
	return "", fmt.Errorf("media type %q has no supported chat input modality", mediaType)
}
