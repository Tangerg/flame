package runtimebinding

import (
	"context"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type feedbackBinding interface {
	CreateFeedback(context.Context, protocol.FeedbackRequest, flameruntime.CommandOptions) error
}

type Feedback struct{ runtime *Connection }

func (f *Feedback) Record(ctx context.Context, signal agent.FeedbackSignal) error {
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
