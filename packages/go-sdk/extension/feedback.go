package extension

type FeedbackDraft struct {
	Source    *ContributionSource `json:"source,omitempty"`
	SessionID string              `json:"sessionId,omitempty"`
	RunID     string              `json:"runId,omitempty"`
	ItemID    string              `json:"itemId,omitempty"`
	Rating    string              `json:"rating,omitempty"`
	Text      string              `json:"text,omitempty"`
}
