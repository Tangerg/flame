package conversation

import (
	"errors"
)

type StreamedText struct{ text string }

func NewStreamedText(text string) StreamedText {
	return StreamedText{text: text}
}

func (s StreamedText) Append(delta BlockDelta) (StreamedText, error) {
	if delta.Text == "" {
		return s, errors.New("streamed text delta is empty")
	}
	return StreamedText{text: s.text + delta.Text}, nil
}

func (s StreamedText) String() string { return s.text }
