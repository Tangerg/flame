package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/sessions"
	feedbackdomain "github.com/Tangerg/flame/runtime/internal/domain/feedback"
	"github.com/Tangerg/flame/runtime/protocol"
)

type feedbackRecorderFake struct {
	command sessions.FeedbackCommand
	err     error
}

func (f *feedbackRecorderFake) Record(_ context.Context, command sessions.FeedbackCommand) error {
	f.command = command
	return f.err
}

func TestCreateFeedbackMapsProtocolRequestToRecorder(t *testing.T) {
	recorder := &feedbackRecorderFake{}
	s := &Handler{feedback: recorder}
	err := s.CreateFeedback(t.Context(), protocol.FeedbackRequest{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1",
		Rating: protocol.FeedbackNegative, Text: "the answer missed the request",
	})
	if err != nil {
		t.Fatalf("CreateFeedback: %v", err)
	}
	if recorder.command != (sessions.FeedbackCommand{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1",
		Rating: feedbackdomain.RatingNegative, Text: "the answer missed the request",
	}) {
		t.Fatalf("command = %+v", recorder.command)
	}
}

func TestCreateFeedbackMapsInvalidEntryToInvalidParams(t *testing.T) {
	s := &Handler{feedback: &feedbackRecorderFake{err: feedbackdomain.ErrInvalid}}
	err := s.CreateFeedback(t.Context(), protocol.FeedbackRequest{})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("CreateFeedback = %v, want invalid params", err)
	}
}
