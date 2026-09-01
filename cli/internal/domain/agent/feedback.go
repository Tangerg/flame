// Feedback values describe user-authored quality signals attached to the
// current agent conversation.
package agent

import (
	"errors"
	"fmt"
	"strings"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

type FeedbackRating string

const (
	FeedbackPositive FeedbackRating = "positive"
	FeedbackNegative FeedbackRating = "negative"
)

func ParseFeedbackRating(value string) (FeedbackRating, error) {
	rating := FeedbackRating(strings.TrimSpace(value))
	if err := rating.Validate(); err != nil {
		return "", err
	}
	return rating, nil
}

func (r FeedbackRating) Validate() error {
	if r != "" && r != FeedbackPositive && r != FeedbackNegative {
		return fmt.Errorf("feedback rating %q is invalid", r)
	}
	return nil
}

type FeedbackSignal struct {
	SessionID string
	RunID     string
	ItemID    string
	Rating    FeedbackRating
	Text      string
}

func (s FeedbackSignal) Validate() error {
	var problems []error
	if s.SessionID != "" {
		if err := runtimeprotocol.ValidateSessionID(s.SessionID); err != nil {
			problems = append(problems, err)
		}
	}
	if s.RunID != "" {
		if err := runtimeprotocol.ValidateRunID(s.RunID); err != nil {
			problems = append(problems, err)
		}
	}
	if s.ItemID != "" {
		if err := runtimeprotocol.ValidateItemID(s.ItemID); err != nil {
			problems = append(problems, err)
		}
	}
	if err := s.Rating.Validate(); err != nil {
		problems = append(problems, err)
	}
	if s.Rating == "" && strings.TrimSpace(s.Text) == "" {
		problems = append(problems, errors.New("feedback requires a rating or text"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("feedback signal: %w", err)
	}
	return nil
}
