package runtimebinding

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type feedbackBinding interface {
	CreateFeedback(context.Context, protocol.FeedbackRequest, protocol.CommandOptions) error
}

type Feedback struct{ runtime *Connection }

func (f *Feedback) Record(ctx context.Context, request protocol.FeedbackRequest) error {
	r := f.runtime
	if err := protocol.ValidateWireTree(request); err != nil {
		return err
	}
	options := r.commandOptions()
	return classifyError(r.feedback.CreateFeedback(ctx, request, options))
}
