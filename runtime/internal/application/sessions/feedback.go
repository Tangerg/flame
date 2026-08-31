package sessions

import (
	"context"
	"time"

	feedbackdomain "github.com/Tangerg/flame/runtime/internal/domain/feedback"
)

// FeedbackCommand is the application input for recording one quality signal.
type FeedbackCommand struct {
	SessionID string
	RunID     string
	ItemID    string
	Rating    feedbackdomain.Rating
	Text      string
}

// FeedbackStore is the durable receiver this feedback use case needs.
type FeedbackStore interface {
	Append(ctx context.Context, entry feedbackdomain.Entry) error
}

// FeedbackRecorder owns the feedback write use case.
type FeedbackRecorder struct {
	store FeedbackStore
}

// NewFeedbackRecorder wires the real durable receiver for feedback records.
func NewFeedbackRecorder(store FeedbackStore) *FeedbackRecorder {
	return &FeedbackRecorder{store: store}
}

// Record validates and appends one immutable feedback observation.
func (r *FeedbackRecorder) Record(ctx context.Context, command FeedbackCommand) error {
	entry, err := feedbackdomain.NewEntry(
		command.SessionID,
		command.RunID,
		command.ItemID,
		command.Rating,
		command.Text,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	return r.store.Append(ctx, entry)
}
