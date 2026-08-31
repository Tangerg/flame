package protocol

// FeedbackRating is the quality signal on feedback.create.
type FeedbackRating string

const (
	FeedbackPositive FeedbackRating = "positive"
	FeedbackNegative FeedbackRating = "negative"
)

// FeedbackRequest is the feedback.create body.
type FeedbackRequest struct {
	SessionID string         `json:"sessionId,omitempty"`
	RunID     string         `json:"runId,omitempty"`
	ItemID    string         `json:"itemId,omitempty"`
	Rating    FeedbackRating `json:"rating,omitempty"` // "positive" | "negative"
	Text      string         `json:"text,omitempty"`
}
