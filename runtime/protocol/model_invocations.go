package protocol

import (
	"github.com/Tangerg/flame/runtime/capability"
	"time"
)

type ListModelEndpointsRequest struct {
	ParentCallID string `json:"parentCallId"`
	Cursor       string `json:"cursor,omitzero"`
	Limit        *int   `json:"limit,omitzero"`
}

type ModelEndpointInvocation struct {
	CostUSD   *float64              `json:"costUSD,omitzero"`
	ID        string                `json:"id"`
	SessionID string                `json:"sessionId,omitzero"`
	RunID     string                `json:"runId,omitzero"`
	Caller    capability.Invocation `json:"caller"`
	Provider  string                `json:"provider"`
	Model     string                `json:"model"`
	Request   string                `json:"request"`
	Response  string                `json:"response,omitzero"`
	Failure   string                `json:"failure,omitzero"`
	State     ModelInvocationState  `json:"state"`
	StartedAt time.Time             `json:"startedAt"`
	SettledAt time.Time             `json:"settledAt,omitzero"`
}

type ModelInvocationState string

const (
	ModelInvocationStarted   ModelInvocationState = "started"
	ModelInvocationCompleted ModelInvocationState = "completed"
	ModelInvocationFailed    ModelInvocationState = "failed"
	ModelInvocationUnknown   ModelInvocationState = "unknown"
)

// ModelInvocationUsage is the provider-reported usage of one call, before Run aggregation.
// The two totals accompany every reported call. A breakdown is present only
// when the provider reports that dimension, so an absent one means unsupported
// rather than zero — the same distinction the provider itself draws.
type ModelInvocationUsage struct {
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CacheReadTokens  *int64 `json:"cacheReadTokens,omitzero"`
	CacheWriteTokens *int64 `json:"cacheWriteTokens,omitzero"`
	ReasoningTokens  *int64 `json:"reasoningTokens,omitzero"`
}

// ModelInvocation is an observed provider attempt. Unknown means recovery could
// not establish its result; SettledAt then records that observation, not a
// provider completion time. Content belongs to Items. Absent usage means it was not recorded, not zero consumption.
type ModelInvocation struct {
	// KnownResponse is the original complete Scope JSON when an outer operation
	// fails after observing inner completion. It is evidence, never continuation.
	KnownResponse            string                `json:"knownResponse,omitzero"`
	FirstOutputLatencyMillis *int64                `json:"firstOutputLatencyMillis,omitzero"`
	Usage                    *ModelInvocationUsage `json:"usage,omitzero"`
	CallID                   string                `json:"callId"`
	RunID                    string                `json:"runId"`
	SegmentID                string                `json:"segmentId"`
	State                    ModelInvocationState  `json:"state"`
	StartedAt                time.Time             `json:"startedAt"`
	SettledAt                time.Time             `json:"settledAt,omitzero"`
}

type ListModelInvocationsRequest struct {
	RunID string `json:"runId"`
	PageQuery
}
