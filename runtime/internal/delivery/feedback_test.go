package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type feedbackRecorderFake struct {
	command extension.FeedbackDraft
	err     error
}

func (f *feedbackRecorderFake) Prepare(_ context.Context, c extension.FeedbackDraft) (func(context.Context) error, error) {
	return func(ctx context.Context) error { return f.Record(ctx, c) }, nil
}

func (f *feedbackRecorderFake) Record(_ context.Context, command extension.FeedbackDraft) error {
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
	if recorder.command != (extension.FeedbackDraft{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1",
		Rating: string(protocol.FeedbackNegative), Text: "the answer missed the request",
	}) {
		t.Fatalf("command = %+v", recorder.command)
	}
}

func TestCreateFeedbackMapsInvalidEntryToInvalidParams(t *testing.T) {
	s := &Handler{feedback: &feedbackRecorderFake{err: protocol.ErrInvalidParams}}
	err := s.CreateFeedback(t.Context(), protocol.FeedbackRequest{})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("CreateFeedback = %v, want invalid params", err)
	}
}
