package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (s *Handler) CreateFeedback(ctx context.Context, in protocol.FeedbackRequest) error {
	return s.feedback.Record(ctx, feedbackCommand(in))
}

func feedbackCommand(in protocol.FeedbackRequest) extension.FeedbackDraft {
	return extension.FeedbackDraft{SessionID: in.SessionID, RunID: in.RunID, ItemID: in.ItemID, Source: in.Source, Rating: string(in.Rating), Text: in.Text}
}
