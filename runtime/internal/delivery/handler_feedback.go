package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/feedback"
	"github.com/Tangerg/flame/runtime/internal/domain/annotation"
)

func (s *Handler) CreateFeedback(ctx context.Context, in protocol.FeedbackRequest) error {
	command, err := feedbackCommand(in)
	if err != nil {
		return err
	}
	return s.feedback.Record(ctx, command)
}

func (s *Handler) commitFeedback(ctx context.Context, in protocol.FeedbackRequest, outcome atomicOutcome) error {
	command, err := feedbackCommand(in)
	if err != nil {
		return err
	}
	write, err := s.feedback.Prepare(ctx, command)
	if err != nil {
		return err
	}
	return outcome.acknowledgement()(ctx, func(tx context.Context) (struct{}, error) {
		return struct{}{}, write(tx)
	})
}

func feedbackCommand(in protocol.FeedbackRequest) (feedback.Command, error) {
	sessionID, err := optionalSessionParameter(in.SessionID)
	if err != nil {
		return feedback.Command{}, err
	}
	runID, err := optionalRunParameter(in.RunID)
	if err != nil {
		return feedback.Command{}, err
	}
	itemID, err := wireIdentity("itemId", in.ItemID, identity.ParseOptionalItem)
	if err != nil {
		return feedback.Command{}, err
	}
	return feedback.Command{
		Subject: annotation.Subject{SessionID: sessionID, RunID: runID, ItemID: itemID},
		Source:  in.Source, Rating: string(in.Rating), Text: in.Text,
	}, nil
}
