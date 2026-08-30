package agent

import (
	"errors"
)

// StreamedText owns the ordered text accumulated for one live block. The
// Runtime publishes no block-position fact: event order is the only exact
// ordering, so the value has one append-only representation.
type StreamedText struct{ text string }

func NewStreamedText(text string) StreamedText {
	return StreamedText{text: text}
}

func (s *StreamedText) Apply(delta BlockDelta) error {
	if delta.Text == "" {
		return errors.New("streamed text delta is empty")
	}
	s.text += delta.Text
	return nil
}

func (s StreamedText) String() string { return s.text }
