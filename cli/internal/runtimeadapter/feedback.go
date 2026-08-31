package runtimeadapter

import (
	"context"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/feedback"
)

type feedbackBinding interface {
	CreateFeedback(context.Context, protocol.FeedbackRequest, flameruntime.CommandOptions) error
}

type feedbackAdapter struct{ runtime *Connection }

var _ feedback.Service = (*feedbackAdapter)(nil)

func (f *feedbackAdapter) Record(ctx context.Context, signal feedback.Signal) error {
	r := f.runtime
	if err := signal.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.feedback.CreateFeedback(ctx, protocol.FeedbackRequest{
		SessionID: signal.SessionID, RunID: signal.RunID, ItemID: signal.ItemID,
		Rating: protocol.FeedbackRating(signal.Rating), Text: signal.Text,
	}, options))
}
