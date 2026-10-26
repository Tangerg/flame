package authoring

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// PreparedInput reads only command-owned values. Plain text is already frozen
// in Message; attachments require the materialized blocks from before dispatch.
// A missing attachment payload must never be reconstructed from its old path.
func (m Message) PreparedInput(input []protocol.ContentBlock) ([]protocol.ContentBlock, error) {
	if input == nil {
		if len(m.Attachments) != 0 {
			return nil, ErrCommandInputUnavailable
		}
		if !m.HasText() {
			return nil, fmt.Errorf("%w: message is empty", ErrCommandInputUnavailable)
		}
		return []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: m.Text}}, nil
	}
	want := len(m.Attachments)
	if m.HasText() {
		want++
	}
	if len(input) != want {
		return nil, fmt.Errorf("%w: content count does not match message", ErrCommandInputUnavailable)
	}
	offset := 0
	if m.HasText() {
		if input[0] != (protocol.ContentBlock{Type: protocol.ContentBlockText, Text: m.Text}) {
			return nil, fmt.Errorf("%w: text does not match message", ErrCommandInputUnavailable)
		}
		offset = 1
	}
	for index, attachment := range m.Attachments {
		block := input[offset+index]
		if block.Type != attachment.Kind || (block.Type == protocol.ContentBlockImage && block.Mime != attachment.MimeType) {
			return nil, fmt.Errorf("%w: attachment %d does not match message", ErrCommandInputUnavailable, index+1)
		}
		if err := protocol.ValidateWireTree(block); err != nil {
			return nil, fmt.Errorf("%w: attachment %d: %w", ErrCommandInputUnavailable, index+1, err)
		}

	}
	return input, nil
}
