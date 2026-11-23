package protocol

import "github.com/Tangerg/flame/packages/go-sdk/extension"

//flame:enum
type FeedbackRating string

const (
	FeedbackPositive FeedbackRating = "positive"
	FeedbackNegative FeedbackRating = "negative"
)

type FeedbackRequest struct {
	Source    *extension.ContributionSource `json:"source,omitempty"`
	SessionID string                        `json:"sessionId,omitempty"`
	RunID     string                        `json:"runId,omitempty"`
	ItemID    string                        `json:"itemId,omitempty"`
	Rating    FeedbackRating                `json:"rating,omitempty"`
	Text      string                        `json:"text,omitempty"`
}
