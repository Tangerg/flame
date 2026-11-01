package main

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestSignalKeepsAuthoredTextAndRequiresMeaning(t *testing.T) {
	for _, in := range []extension.FeedbackDraft{{}, {Text: "  "}, {Rating: "unknown"}, {SessionID: "has space", Text: "text"}} {
		if _, err := prepare(in); !errors.Is(err, errEmptySignal) {
			t.Fatalf("invalid feedback %+v: %v", in, err)
		}
	}
	for _, in := range []extension.FeedbackDraft{{Rating: string(protocol.FeedbackNegative)}, {Rating: string(protocol.FeedbackPositive), Text: " text "}, {Text: "\n  text only  "}} {
		got, err := prepare(in)
		if err != nil || string(got.Rating) != in.Rating || got.Text != in.Text {
			t.Fatalf("changed observation %+v %v", got, err)
		}
	}
}
