package protocol

import "time"

type ModelInvocationState string

const (
	ModelInvocationStarted   ModelInvocationState = "started"
	ModelInvocationCompleted ModelInvocationState = "completed"
	ModelInvocationFailed    ModelInvocationState = "failed"
	ModelInvocationUnknown   ModelInvocationState = "unknown"
)

// ModelInvocationUsage is the provider-reported usage of one call, before Run aggregation.
type ModelInvocationUsage struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	ReasoningTokens  int64 `json:"reasoningTokens"`
}

// ModelInvocation is an observed provider attempt. Unknown means recovery could
// not establish its result; SettledAt then records that observation, not a
// provider completion time. Content belongs to Items. Absent usage means it was not recorded, not zero consumption.
type ModelInvocation struct {
	Usage     *ModelInvocationUsage `json:"usage,omitempty"`
	CallID    string                `json:"callId"`
	RunID     string                `json:"runId"`
	SegmentID string                `json:"segmentId"`
	State     ModelInvocationState  `json:"state"`
	StartedAt time.Time             `json:"startedAt"`
	SettledAt time.Time             `json:"settledAt,omitzero"`
}

type ListModelInvocationsRequest struct {
	RunID string `json:"runId"`
	PageQuery
}
