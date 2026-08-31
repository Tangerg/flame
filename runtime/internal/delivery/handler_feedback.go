package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	feedbackdomain "github.com/Tangerg/flame/runtime/internal/domain/session/feedback"
	"github.com/Tangerg/flame/runtime/protocol"
)

// CreateFeedback records an ungated quality signal in the runtime's durable
// feedback ledger. The write-only protocol shape intentionally has no readback,
// but a successful ack always means the application receiver accepted it.
func (s *Handler) CreateFeedback(ctx context.Context, in protocol.FeedbackRequest) error {
	err := s.feedback.Record(ctx, sessions.FeedbackCommand{
		SessionID: in.SessionID,
		RunID:     in.RunID,
		ItemID:    in.ItemID,
		Rating:    feedbackdomain.Rating(in.Rating),
		Text:      in.Text,
	})
	if errors.Is(err, feedbackdomain.ErrInvalid) {
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	return err
}
