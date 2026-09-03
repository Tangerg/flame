package runtimebinding

import (
	"context"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type feedbackBinding interface {
	CreateFeedback(context.Context, protocol.FeedbackRequest, flameruntime.CommandOptions) error
}

type Feedback struct{ runtime *Connection }

func (f *Feedback) Record(ctx context.Context, request protocol.FeedbackRequest) error {
	r := f.runtime
	if err := request.ValidateWire(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.feedback.CreateFeedback(ctx, request, options))
}
