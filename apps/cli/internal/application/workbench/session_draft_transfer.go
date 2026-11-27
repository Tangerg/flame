package workbench

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func mergeSessionDraft(existing, incoming authoring.Message) (authoring.Message, error) {
	values := []struct {
		label   string
		message authoring.Message
	}{
		{label: "existing", message: existing},
		{label: "incoming", message: incoming},
	}
	for _, value := range values {
		if value.message.IsEmpty() {
			continue
		}
		if err := value.message.Validate(); err != nil {
			return authoring.Message{}, fmt.Errorf("%s session draft: %w", value.label, err)
		}
	}
	if existing.IsEmpty() {
		return incoming.Clone(), nil
	}
	if incoming.IsEmpty() {
		return existing.Clone(), nil
	}

	merged := existing.Clone()
	switch {
	case strings.TrimSpace(merged.Text) == "":
		merged.Text = incoming.Text
	case strings.TrimSpace(incoming.Text) == "", merged.Text == incoming.Text:
	default:
		merged.Text += "\n\n" + incoming.Text
	}
	byID := make(map[string]authoring.Attachment, len(merged.Attachments)+len(incoming.Attachments))
	byPath := make(map[string]authoring.Attachment, len(merged.Attachments)+len(incoming.Attachments))
	for _, item := range merged.Attachments {
		byID[item.ID] = item
		byPath[item.Path] = item
	}
	for _, item := range incoming.Attachments {
		if current, found := byID[item.ID]; found {
			if current != item {
				return authoring.Message{}, fmt.Errorf("session drafts conflict on attachment id %q", item.ID)
			}
			continue
		}
		if current, found := byPath[item.Path]; found {
			if current != item {
				return authoring.Message{}, fmt.Errorf("session drafts conflict on attachment path %q", item.Path)
			}
			continue
		}
		merged.Attachments = append(merged.Attachments, item)
		byID[item.ID] = item
		byPath[item.Path] = item
	}
	if err := merged.Validate(); err != nil {
		return authoring.Message{}, fmt.Errorf("merged session draft: %w", err)
	}
	return merged, nil
}

func (s *Store) TransferSessionDraft(sourceID, destinationID string, remaining authoring.Message) error {
	if err := protocol.ValidateSessionID(sourceID); err != nil {
		return fmt.Errorf("draft transfer source: %w", err)
	}
	if err := protocol.ValidateSessionID(destinationID); err != nil {
		return fmt.Errorf("draft transfer destination: %w", err)
	}
	if sourceID == destinationID {
		return errors.New("draft transfer source and destination are the same session")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	source := s.sessionUpdate(sourceID)
	if source.draft.Equal(remaining) {
		return nil
	}
	destination := s.sessionUpdate(destinationID)
	merged, err := mergeSessionDraft(destination.draft, source.draft)
	if err != nil {
		return fmt.Errorf("merge transferred draft: %w", err)
	}
	source.draft = remaining.Clone()
	destination.draft = merged
	return s.commitSessions([]sessionUpdate{destination, source})
}
