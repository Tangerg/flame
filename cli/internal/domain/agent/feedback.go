// Feedback values describe user-authored quality signals attached to the
// current agent conversation.
package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

func ParseFeedbackRating(value string) (protocol.FeedbackRating, error) {
	rating := protocol.FeedbackRating(strings.TrimSpace(value))
	if err := (protocol.FeedbackRequest{Rating: rating}).ValidateWire(); err != nil {
		return "", err
	}
	return rating, nil
}

type FeedbackSignal struct {
	SessionID string
	RunID     string
	ItemID    string
	Rating    protocol.FeedbackRating
	Text      string
}

func (s FeedbackSignal) Validate() error {
	var problems []error
	if err := protocol.ValidateWireTree(protocol.FeedbackRequest{
		SessionID: s.SessionID, RunID: s.RunID, ItemID: s.ItemID,
		Rating: s.Rating, Text: s.Text,
	}); err != nil {
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
