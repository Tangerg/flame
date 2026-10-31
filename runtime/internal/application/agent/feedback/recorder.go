package feedback

import (
	"context"
	"errors"
	"time"

	feedbackdomain "github.com/Tangerg/flame/runtime/internal/domain/feedback"
	"github.com/samber/lo"
)

// Command is the application input for recording one quality signal.
type Command struct {
	SessionID string
	RunID     string
	ItemID    string
	Rating    feedbackdomain.Rating
	Text      string
}

// store is the durable receiver this feedback use case needs.
type store interface {
	Append(ctx context.Context, entry feedbackdomain.Entry) error
}

// Recorder owns the feedback write use case.
type Recorder struct {
	store store
	now   func() time.Time
}

// NewRecorder wires the complete durable receiver for feedback records.
func NewRecorder(store store, now func() time.Time) (*Recorder, error) {
	if lo.IsNil(store) {
		return nil, errors.New("feedback: store is required")
	}
	if now == nil {
		return nil, errors.New("feedback: clock is required")
	}
	return &Recorder{store: store, now: now}, nil
}

// Record validates and appends one immutable feedback observation.
func (r *Recorder) Record(ctx context.Context, command Command) error {
	entry, err := feedbackdomain.NewEntry(
		command.SessionID,
		command.RunID,
		command.ItemID,
		command.Rating,
		command.Text,
		r.now().UTC(),
	)
	if err != nil {
		return err
	}
	return r.store.Append(ctx, entry)
}
