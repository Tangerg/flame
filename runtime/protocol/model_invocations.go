package protocol

import "time"

type ModelInvocationState string

const (
	ModelInvocationStarted   ModelInvocationState = "started"
	ModelInvocationCompleted ModelInvocationState = "completed"
	ModelInvocationFailed    ModelInvocationState = "failed"
	ModelInvocationUnknown   ModelInvocationState = "unknown"
)

// ModelInvocation is an observed provider attempt. Unknown means recovery could
// not establish its result; SettledAt then records that observation, not a
// provider completion time. Content and accounting belong to Items and Runs.
type ModelInvocation struct {
	CallID    string               `json:"callId"`
	RunID     string               `json:"runId"`
	SegmentID string               `json:"segmentId"`
	State     ModelInvocationState `json:"state"`
	StartedAt time.Time            `json:"startedAt"`
	SettledAt time.Time            `json:"settledAt,omitzero"`
}

type ListModelInvocationsRequest struct {
	RunID string `json:"runId"`
	PageQuery
}
