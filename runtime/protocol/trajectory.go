package protocol

import "time"

type TrajectoryEntryType string

const (
	TrajectoryEntryRun      TrajectoryEntryType = "run"
	TrajectoryEntryModel    TrajectoryEntryType = "model"
	TrajectoryEntryEndpoint TrajectoryEntryType = "endpoint"
	TrajectoryEntryItem     TrajectoryEntryType = "item"
)

// TrajectoryEntry projects one durable source record at read time. OccurredAt
// orders source observations; it is not a global event sequence or a causal edge.
type TrajectoryEntry struct {
	Type       TrajectoryEntryType      `json:"type"`
	OccurredAt time.Time                `json:"occurredAt"`
	Run        *RunRef                  `json:"run,omitzero"`
	Model      *ModelInvocation         `json:"model,omitzero"`
	Endpoint   *ModelEndpointInvocation `json:"endpoint,omitzero"`
	Item       *Item                    `json:"item,omitzero"`
}

type ListSessionTrajectoryRequest struct {
	SessionID          string `json:"sessionId"`
	IncludeDescendants bool   `json:"includeDescendants,omitzero"`
	PageQuery
}
