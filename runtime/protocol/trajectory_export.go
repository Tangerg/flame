package protocol

import (
	"encoding/json/jsontext"
	"errors"
	"time"
)

const SessionTrajectoryVersion = 2

var ErrExportTooLarge = errors.New("export_too_large")

type ExportTrajectoryRequest struct {
	SessionID string `json:"sessionId"`
}

type ExportTrajectoryResponse struct {
	Trajectory SessionTrajectory `json:"trajectory"`
}

// SessionTrajectory contains the complete retained evidence at one idle Session
// snapshot. It is an evaluation document, not an importable execution checkpoint.
type SessionTrajectory struct {
	SchemaVersion    int                  `json:"schemaVersion"`
	CollectedAt      time.Time            `json:"collectedAt"`
	Session          Session              `json:"session"`
	Runs             []RunRef             `json:"runs"`
	Items            []Item               `json:"items"`
	Messages         []jsontext.Value     `json:"messages"`
	ToolResults      []ArtifactToolResult `json:"toolResults"`
	Plan             []PlanStep           `json:"plan"`
	ModelInvocations []ModelInvocation    `json:"modelInvocations"`
	ToolAttempts     []ToolAttempt        `json:"toolAttempts"`
	Feedback         []FeedbackEntry      `json:"feedback"`
	Limitations      []string             `json:"limitations"`
}

type ToolAttemptState string

const (
	ToolAttemptStarted    ToolAttemptState = "started"
	ToolAttemptCompleted  ToolAttemptState = "completed"
	ToolAttemptIncomplete ToolAttemptState = "incomplete"
)

// Incomplete includes suspension for input; it does not establish tool failure.
// A retained started attempt has no known settlement, even on a finished Run.
type ToolAttempt struct {
	CallID    string           `json:"callId"`
	ItemID    string           `json:"itemId"`
	RunID     string           `json:"runId"`
	SegmentID string           `json:"segmentId"`
	State     ToolAttemptState `json:"state"`
	StartedAt time.Time        `json:"startedAt"`
	SettledAt time.Time        `json:"settledAt,omitzero"`
}

type FeedbackEntry struct {
	SessionID string         `json:"sessionId,omitempty"`
	RunID     string         `json:"runId,omitempty"`
	ItemID    string         `json:"itemId,omitempty"`
	Rating    FeedbackRating `json:"rating,omitempty"`
	Text      string         `json:"text,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}
